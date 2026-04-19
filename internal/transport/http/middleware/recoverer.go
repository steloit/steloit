package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5/middleware"
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

// internalErrorBody is the canonical 500 envelope written by the
// recoverer when a panic is caught. Hand-written constant rather than a
// runtime AppError construction — the body is fixed, the path is hot,
// and the recoverer must stay functional through the pkg/response
// rewrite that lands later in the Chi migration.
const internalErrorBody = `{"success":false,"error":{"type":"api_error","code":"api_error","message":"Internal server error"}}`

// writePanicResponse emits the canonical 500 envelope.
func writePanicResponse(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(internalErrorBody))
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
