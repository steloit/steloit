package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"brokle/internal/version"
)

// Server bundles the chi router, the two Huma API instances, the
// http.Server, and the readyState used by the two-phase drain. Use
// New to construct one and Start / Shutdown / ServeErr to drive it.
//
// The struct is the package's only exported handle; the addRoutes /
// newAPI* / readyState plumbing is intentionally package-private so
// callers can't accidentally re-wire half the stack.
type Server struct {
	deps   Deps
	mux    *chi.Mux
	api    apiPair // {public, admin}
	http   *http.Server
	listen net.Listener
	ready  *readyState
	errCh  chan error
}

// New constructs a Server. Building the chi mux, the two huma.API
// instances, and wiring the route table happens here so any
// misconfiguration (jub0bs/cors panics, duplicate route registration)
// fails at boot rather than first request.
//
// Caller is responsible for invoking Start to bind the listener and
// Shutdown to drain. Returns an error only when the listener bind
// fails or http.Server config is rejected; route-tree assembly never
// returns an error (it panics on misconfig — see addRoutes / mustCORS).
func New(deps Deps) (*Server, error) {
	if deps.Logger == nil {
		return nil, errors.New("server: Deps.Logger is required")
	}
	if deps.Config == nil {
		return nil, errors.New("server: Deps.Config is required")
	}

	mux := chi.NewRouter()
	apiPublic := newAPIPublic(mux, version.Get())
	apiAdmin := newAPIAdmin(mux, version.Get())

	ready := newReadyState()
	addRoutes(mux, apiPublic, apiAdmin, deps, ready)

	s := &Server{
		deps:  deps,
		mux:   mux,
		api:   apiPair{Public: apiPublic, Admin: apiAdmin},
		ready: ready,
		errCh: make(chan error, 1),
	}

	addr := fmt.Sprintf(":%d", deps.Config.Server.Port)
	s.http = &http.Server{
		Addr:    addr,
		Handler: mux,
		// Tier-1 production defaults. ReadHeaderTimeout is the
		// load-bearing one — without it, a malicious client that
		// dribbles headers indefinitely (Slowloris) holds connections
		// forever. golangci-lint G112 flags missing values.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Duration(deps.Config.Server.ReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(deps.Config.Server.WriteTimeout) * time.Second,
		IdleTimeout:       time.Duration(deps.Config.Server.IdleTimeout) * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}

	return s, nil
}

// Start binds the listener and spawns the Serve goroutine. Returns
// once the listener is bound; Serve runs in the background. Any
// post-bind error (Serve returning a non-ErrServerClosed error) is
// surfaced via ServeErr.
//
// Marks /readyz as ready as the final step so the LB only routes
// traffic to a fully-wired pod.
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("server: bind %s: %w", s.http.Addr, err)
	}
	s.listen = lis
	s.deps.Logger.Info("HTTP server listening", "addr", s.http.Addr)

	go func() {
		if err := s.http.Serve(lis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.errCh <- err
		}
	}()

	s.ready.MarkReady()
	return nil
}

// ServeErr exposes the post-bind serve error channel — a single
// receive on this channel returns the error that took down the
// listener (or never resolves if the server stays healthy until
// Shutdown is called).
func (s *Server) ServeErr() <-chan error { return s.errCh }

// Shutdown performs the canonical two-phase drain:
//
//  1. Flip /readyz to 503 immediately, so the load balancer's next
//     readiness probe (typically within 5–10 s) deregisters the pod.
//  2. Sleep drainPause to give the LB time to drain in-flight
//     traffic and stop sending new requests. This is the single
//     most-commonly-missed production gotcha — without it, k8s
//     keeps routing to a shutting-down pod for one full readiness
//     interval.
//  3. Call srv.Shutdown(ctx) which closes the listener and waits
//     for active connections to finish (or the supplied ctx
//     deadline expires, whichever comes first).
//
// The supplied ctx bounds the total drain — pass a context with a
// timeout sized to your longest acceptable in-flight request (we
// recommend 30 s for HTTP APIs, longer for streaming endpoints).
func (s *Server) Shutdown(ctx context.Context) error {
	s.deps.Logger.Info("HTTP server shutdown: phase 1, marking unready")
	s.ready.MarkNotReady()

	const drainPause = 5 * time.Second
	select {
	case <-time.After(drainPause):
	case <-ctx.Done():
		// Caller's context expired before LB drain — proceed straight
		// to Shutdown so the deadline isn't double-charged.
		s.deps.Logger.Warn("HTTP server shutdown: drain pause cancelled by ctx",
			"err", ctx.Err())
	}

	s.deps.Logger.Info("HTTP server shutdown: phase 2, closing listener and draining connections")
	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("server: graceful shutdown failed: %w", err)
	}
	s.deps.Logger.Info("HTTP server shutdown: complete")
	return nil
}

// Handler returns the underlying chi mux. Exposed for tests that
// drive the server via httptest.NewRecorder without a real
// listener.
func (s *Server) Handler() http.Handler { return s.mux }

// apiPair holds the two huma.API instances side by side. Keeping
// them in one struct (rather than two separate fields on Server)
// makes the "two-surface" architecture visible at a glance and lets
// future code iterate over both with
// `for _, a := range []huma.API{p.Public, p.Admin}`.
type apiPair struct {
	Public, Admin huma.API
}
