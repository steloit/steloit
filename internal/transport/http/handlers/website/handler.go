// Package website wires the marketing-site contact form endpoint
// onto the dashboard Huma API surface (apiAdmin). The package
// follows the canonical chi+Huma per-domain registration pattern
// established for the migration: a RegisterRoutes function takes
// the API instance plus the explicit services it needs, and each
// route is registered as a Huma operation with typed input/output
// structs.
//
// This is the first vertical slice of the gin → Huma handler
// conversion (Step 4 of the chi+Huma migration). Subsequent
// domains follow the same shape: prefix constant + RegisterRoutes;
// per-operation files (or a single handler file for small domains
// like this one) hold the input, output, and handler implementations.
package website

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"brokle/internal/core/domain/website"
	"brokle/internal/transport/http/httpctx"
)

// prefix is the dashboard-plane base path for website routes. Lives
// at the package level so every operation in this package shares
// one mutable point of truth — chi sub-router prefixes are NOT used
// because they confuse Huma's OpenAPI emission (Huma discussion
// #589). Operations declare the full path inline.
const prefix = "/api/v1/website"

// handler bundles the website service + logger so the operation
// methods don't carry them on every signature. The struct is
// package-private; the only public surface is RegisterRoutes.
type handler struct {
	svc    website.WebsiteService
	logger *slog.Logger
}

// RegisterRoutes registers every website operation on the supplied
// huma.API. Should be called against the apiAdmin instance only —
// the marketing form submits cross-origin from the public website
// to the dashboard plane (it is not part of the SDK contract).
func RegisterRoutes(api huma.API, svc website.WebsiteService, logger *slog.Logger) {
	h := &handler{
		svc:    svc,
		logger: logger,
	}

	huma.Register(api, huma.Operation{
		OperationID:   "submit-contact-form",
		Method:        http.MethodPost,
		Path:          prefix + "/contact",
		Tags:          []string{"website"},
		Summary:       "Submit a contact form",
		Description:   "Public endpoint accepting a contact form submission from the marketing site. No authentication required; rate-limited by IP at the route-group level.",
		DefaultStatus: http.StatusCreated,
	}, h.submitContact)
}

// SubmitContactInput is the typed request shape Huma uses to
// validate the request body before invoking the handler. Validation
// tags map to OpenAPI 3.1 constraints + runtime checks; failures
// surface as 422 validation_error responses through the apiResponse
// envelope override.
type SubmitContactInput struct {
	Body submitContactBody
}

// submitContactBody is the JSON body schema. Field tags carry the
// same constraints the gin handler enforced via `binding:` tags;
// Huma reads them from the `minLength`/`maxLength`/`format` form
// for both validation and OpenAPI doc emission.
type submitContactBody struct {
	Name        string `json:"name" minLength:"2" maxLength:"255" doc:"Contact name"`
	Email       string `json:"email" format:"email" maxLength:"255" doc:"Contact email"`
	Company     string `json:"company,omitempty" maxLength:"255" doc:"Optional company name"`
	Subject     string `json:"subject" minLength:"5" maxLength:"255" doc:"Subject line"`
	Message     string `json:"message" minLength:"10" maxLength:"5000" doc:"Message body"`
	InquiryType string `json:"inquiry_type,omitempty" maxLength:"50" doc:"Optional inquiry classification (sales, support, partnership, …)"`
}

// SubmitContactOutput is the typed response. The Body field is what
// Huma serialises into the response envelope.
type SubmitContactOutput struct {
	Body submitContactResponse
}

// submitContactResponse is the response shape returned to the
// browser. Carries a human-readable confirmation message safe to
// render verbatim.
type submitContactResponse struct {
	Message string `json:"message" doc:"Confirmation message safe to render verbatim"`
}

// submitContact is the Huma operation handler. Receives the typed
// input pre-validated by Huma, calls the service, and returns the
// typed output. Errors flow through verbatim — the package-level
// huma.NewError override (server/api_error.go) maps AppError values
// to the canonical APIResponse envelope, so service errors carry
// their Type / Code / Message / Details intact.
//
// IP + User-Agent come from the request context via httpctx —
// populated by the RequestMetadata middleware in the global chain
// (internal/server/routes.go). Returns "" for each when the
// middleware didn't run (tests calling the handler directly
// without a full HTTP chain); the audit row records the empty
// value rather than crashing.
func (h *handler) submitContact(ctx context.Context, in *SubmitContactInput) (*SubmitContactOutput, error) {
	req := &website.CreateContactSubmissionRequest{
		Name:        in.Body.Name,
		Email:       in.Body.Email,
		Company:     in.Body.Company,
		Subject:     in.Body.Subject,
		Message:     in.Body.Message,
		InquiryType: in.Body.InquiryType,
	}

	if err := h.svc.SubmitContactForm(ctx, req, httpctx.ClientIP(ctx), httpctx.UserAgent(ctx)); err != nil {
		h.logger.WarnContext(ctx, "contact-form submission rejected",
			"error", err,
			"email", req.Email,
			"subject", req.Subject,
		)
		return nil, err
	}

	return &SubmitContactOutput{
		Body: submitContactResponse{
			Message: "Thank you for your message. We'll get back to you soon.",
		},
	}, nil
}
