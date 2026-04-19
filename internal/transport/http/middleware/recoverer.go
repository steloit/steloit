package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5/middleware"

	appErrors "brokle/pkg/errors"
)

// Recoverer is the chi-shape panic recovery middleware that replaces the
// Gin-based Recovery in middleware.go.
//
// It deliberately uses log/slog (not the stdlib log that chi's built-in
// Recoverer falls back to) so panic records are emitted with the same
// structured fields as the rest of the application — request_id,
// method, path, status — and join cleanly into log aggregation pipelines.
//
// http.ErrAbortHandler is re-raised verbatim per chi issue #588: the
// stdlib documents this sentinel as the explicit way for a handler to
// abort a connection (used by SSE/WebSocket on client disconnect, and by
// chi's own middleware.Timeout). Treating it as a recovered panic would
// log spurious "panic" lines for every dropped streaming connection.
//
// The 500 envelope is written via the AppError pipeline so the response
// shape matches non-panic error paths and Huma's renderer in
// internal/transport/http/api_error.go.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				// Re-raise ErrAbortHandler so chi/middleware.Timeout and
				// streaming-handler aborts propagate without polluting the
				// log with fake panic stacks.
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}

				logger.ErrorContext(r.Context(), "panic recovered in HTTP handler",
					"panic", rec,
					"stack", string(debug.Stack()),
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", middleware.GetReqID(r.Context()),
					"remote_addr", r.RemoteAddr,
				)

				// Don't try to write a body if headers are already on the
				// wire — the connection state is past the point of useful
				// recovery and writing again would log a "superfluous
				// WriteHeader call" warning.
				if !headersWritten(w) {
					writePanicResponse(w, r)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// writePanicResponse emits a 500 envelope shaped like every other error
// response in the codebase — uses appErrors.NewInternalError so a future
// rev of the envelope only needs to change in one place.
func writePanicResponse(w http.ResponseWriter, _ *http.Request) {
	err := appErrors.NewInternalError("Internal server error", nil)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.StatusCode)
	// Hand-write the envelope here rather than reach into pkg/response —
	// the response package will be rewritten in a later step and this
	// recoverer must remain functional through that transition. The shape
	// is the documented APIResponse envelope:
	//   {"success":false,"error":{"code":"INTERNAL_ERROR","message":"Internal server error","type":"INTERNAL_ERROR"}}
	const body = `{"success":false,"error":{"code":"INTERNAL_ERROR","message":"Internal server error","type":"INTERNAL_ERROR"}}`
	_, _ = w.Write([]byte(body))
}

// headersWritten reports whether the response headers have already been
// flushed. chi's WrapResponseWriter exposes Status() — anything non-zero
// means WriteHeader was called. Vanilla http.ResponseWriter (in tests
// without the wrapper) returns false.
func headersWritten(w http.ResponseWriter) bool {
	if ww, ok := w.(middleware.WrapResponseWriter); ok {
		return ww.Status() != 0
	}
	return false
}
