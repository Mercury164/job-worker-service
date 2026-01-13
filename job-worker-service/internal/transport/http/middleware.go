package httptransport

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()

		// chi middleware.RequestID кладёт id в контекст
		reqID := middleware.GetReqID(r.Context())

		next.ServeHTTP(sw, r)

		log.Printf("[http] req_id=%s method=%s path=%s status=%d bytes=%d duration_ms=%d",
			reqID,
			r.Method,
			r.URL.Path,
			sw.status,
			sw.bytes,
			time.Since(start).Milliseconds(),
		)
	})
}
