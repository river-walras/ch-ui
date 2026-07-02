package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recoverer recovers from panics in HTTP handlers, logs the panic with a stack
// trace, and returns a clean 500 JSON response instead of letting the
// connection drop silently. Without this, a nil-dereference in any handler
// produces an opaque dropped connection with no structured log line.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// http.ErrAbortHandler is the documented way to abort a response
				// without logging noise; let it propagate.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				slog.Error("Recovered from panic in HTTP handler",
					"panic", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": "Internal server error",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
