package compliance

import "context"

// Compliance interface that both stub and real implementation satisfy
type Compliance interface {
	ValidateCompliance(ctx context.Context, data any) error
	GenerateAuditReport(ctx context.Context) ([]byte, error)
	AnonymizePII(ctx context.Context, data any) (any, error)
	CheckSOC2Compliance(ctx context.Context) (bool, error)
	CheckHIPAACompliance(ctx context.Context) (bool, error)
	CheckGDPRCompliance(ctx context.Context) (bool, error)
}

// StubCompliance provides stub implementation for OSS version
type StubCompliance struct{}

// New returns the compliance implementation (stub or real based on build tags)
func New() Compliance {
	return &StubCompliance{}
}

func (s *StubCompliance) ValidateCompliance(ctx context.Context, data any) error {
	// Stub: Always passes validation
	return nil
}

func (s *StubCompliance) GenerateAuditReport(ctx context.Context) ([]byte, error) {
	// Stub: Returns basic report
	return []byte("Basic audit report - Enterprise license required for advanced compliance features"), nil
}

func (s *StubCompliance) AnonymizePII(ctx context.Context, data any) (any, error) {
	// Stub: Returns data unchanged
	return data, nil
}

func (s *StubCompliance) CheckSOC2Compliance(ctx context.Context) (bool, error) {
	// Stub: Always returns false for compliance checks
	return false, nil
}

func (s *StubCompliance) CheckHIPAACompliance(ctx context.Context) (bool, error) {
	// Stub: Always returns false for compliance checks
	return false, nil
}

func (s *StubCompliance) CheckGDPRCompliance(ctx context.Context) (bool, error) {
	// Stub: Always returns false for compliance checks
	return false, nil
}
