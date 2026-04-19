package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"brokle/internal/config"
	"brokle/internal/server"
	"brokle/pkg/logging"
)

// App represents the main application
type App struct {
	config       *config.Config
	logger       *slog.Logger
	providers    *ProviderContainer
	httpServer   *server.Server
	mode         DeploymentMode
	shutdownOnce sync.Once
}

func NewServer(cfg *config.Config) (*App, error) {
	logger := logging.NewLoggerWithFormat(
		logging.ParseLevel(cfg.Logging.Level),
		cfg.Logging.Format,
	)

	core, err := ProvideCore(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize core: %w", err)
	}

	core.Services = ProvideServerServices(core)
	core.Enterprise = ProvideEnterpriseServices(cfg, logger)

	server, err := ProvideServer(core)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize server: %w", err)
	}

	return &App{
		mode:       ModeServer,
		config:     cfg,
		logger:     logger,
		httpServer: server.HTTPServer,
		providers: &ProviderContainer{
			Core:    core,
			Server:  server,
			Workers: nil,
			Mode:    ModeServer,
		},
	}, nil
}

func NewWorker(cfg *config.Config) (*App, error) {
	logger := logging.NewLoggerWithFormat(
		logging.ParseLevel(cfg.Logging.Level),
		cfg.Logging.Format,
	)

	core, err := ProvideCore(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize core: %w", err)
	}

	core.Services = ProvideWorkerServices(core)
	core.Enterprise = nil

	workers, err := ProvideWorkers(core)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize workers: %w", err)
	}

	return &App{
		mode:       ModeWorker,
		config:     cfg,
		logger:     logger,
		httpServer: nil,
		providers: &ProviderContainer{
			Core:    core,
			Server:  nil,
			Workers: workers,
			Mode:    ModeWorker,
		},
	}, nil
}

func (a *App) Start() error {
	a.logger.Info("Starting Brokle Platform...", "mode", a.mode)

	switch a.mode {
	case ModeServer:
		var g errgroup.Group

		g.Go(func() error {
			return a.httpServer.Start()
		})

		g.Go(func() error {
			return a.providers.Server.GRPCServer.Start()
		})

		if err := g.Wait(); err != nil {
			return err
		}

		a.logger.Info("Brokle Platform started successfully")

		go func() {
			select {
			case err := <-a.httpServer.ServeErr():
				a.logger.Error("HTTP server failed unexpectedly", "error", err)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				_ = a.Shutdown(ctx)
				os.Exit(1)

			case err := <-a.providers.Server.GRPCServer.ServeErr():
				a.logger.Error("gRPC server failed unexpectedly", "error", err)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				_ = a.Shutdown(ctx)
				os.Exit(1)
			}
		}()

	case ModeWorker:
		if err := a.providers.Workers.TelemetryConsumer.Start(context.Background()); err != nil {
			a.logger.Error("Failed to start telemetry stream consumer", "error", err)
			return err
		}
		a.logger.Info("Telemetry stream consumer started")

		// Start evaluator worker
		if a.providers.Workers.EvaluatorWorker != nil {
			if err := a.providers.Workers.EvaluatorWorker.Start(context.Background()); err != nil {
				a.logger.Error("Failed to start evaluator worker", "error", err)
				return err
			}
			a.logger.Info("Evaluator worker started")
		}

		// Start evaluation worker
		if a.providers.Workers.EvaluationWorker != nil {
			if err := a.providers.Workers.EvaluationWorker.Start(context.Background()); err != nil {
				a.logger.Error("Failed to start evaluation worker", "error", err)
				return err
			}
			a.logger.Info("Evaluation worker started")
		}

		if a.providers.Workers.ManualTriggerWorker != nil {
			if err := a.providers.Workers.ManualTriggerWorker.Start(context.Background()); err != nil {
				a.logger.Error("Failed to start manual trigger worker", "error", err)
				return err
			}
			a.logger.Info("Manual trigger worker started")
		}

		// Start usage aggregation worker (syncs ClickHouse usage to PostgreSQL billing)
		if a.providers.Workers.UsageAggregationWorker != nil {
			a.providers.Workers.UsageAggregationWorker.Start()
			a.logger.Info("Usage aggregation worker started")
		}

		// Start contract expiration worker (daily job to expire contracts past end_date)
		if a.providers.Workers.ContractExpirationWorker != nil {
			a.providers.Workers.ContractExpirationWorker.Start()
			a.logger.Info("Contract expiration worker started")
		}

		// Start annotation lock expiry worker (every minute, releases stale locks)
		if a.providers.Workers.LockExpiryWorker != nil {
			a.providers.Workers.LockExpiryWorker.Start()
			a.logger.Info("Annotation lock expiry worker started")
		}
	}

	return nil
}

func (a *App) Shutdown(ctx context.Context) error {
	var shutdownErr error

	a.shutdownOnce.Do(func() {
		shutdownErr = a.doShutdown(ctx)
	})

	return shutdownErr
}

func (a *App) doShutdown(ctx context.Context) error {
	a.logger.Info("Shutting down Brokle Platform...", "mode", a.mode)

	var wg sync.WaitGroup

	switch a.mode {
	case ModeServer:
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.providers.Server.GRPCServer.Shutdown(ctx); err != nil {
				a.logger.Error("Failed to shutdown gRPC server", "error", err)
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			if a.httpServer != nil {
				if err := a.httpServer.Shutdown(ctx); err != nil {
					a.logger.Error("Failed to shutdown HTTP server", "error", err)
				}
			}
		}()

	case ModeWorker:
		wg.Add(1)
		go func() {
			defer wg.Done()
			if a.providers.Workers != nil {
				if a.providers.Workers.TelemetryConsumer != nil {
					a.providers.Workers.TelemetryConsumer.Stop()
				}
				if a.providers.Workers.EvaluatorWorker != nil {
					a.providers.Workers.EvaluatorWorker.Stop()
				}
				if a.providers.Workers.EvaluationWorker != nil {
					a.providers.Workers.EvaluationWorker.Stop()
				}
				if a.providers.Workers.ManualTriggerWorker != nil {
					a.providers.Workers.ManualTriggerWorker.Stop()
				}
				if a.providers.Workers.UsageAggregationWorker != nil {
					a.providers.Workers.UsageAggregationWorker.Stop()
				}
				if a.providers.Workers.ContractExpirationWorker != nil {
					a.providers.Workers.ContractExpirationWorker.Stop()
				}
				if a.providers.Workers.LockExpiryWorker != nil {
					a.providers.Workers.LockExpiryWorker.Stop()
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if a.providers != nil {
			if err := a.providers.Shutdown(); err != nil {
				a.logger.Error("Failed to shutdown providers", "error", err)
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		a.logger.Info("Brokle Platform shutdown completed")
		return nil
	case <-ctx.Done():
		a.logger.Warn("Shutdown timeout exceeded, forcing shutdown")
		return ctx.Err()
	}
}

// GetProviders returns the provider container for access to all services and dependencies
func (a *App) GetProviders() *ProviderContainer {
	return a.providers
}

// Health returns the health status of all components using providers
func (a *App) Health() map[string]string {
	if a.providers != nil {
		return a.providers.HealthCheck()
	}

	return map[string]string{
		"status": "providers not initialized",
	}
}

// GetWorkers returns the worker container for background processing
func (a *App) GetWorkers() *WorkerContainer {
	if a.providers == nil {
		return nil
	}
	return a.providers.Workers
}

// GetLogger returns the application logger
func (a *App) GetLogger() *slog.Logger {
	return a.logger
}

// GetConfig returns the application configuration
func (a *App) GetConfig() *config.Config {
	return a.config
}

// GetDatabases returns the database connections
func (a *App) GetDatabases() *DatabaseContainer {
	if a.providers == nil || a.providers.Core == nil {
		return nil
	}
	return a.providers.Core.Databases
}
