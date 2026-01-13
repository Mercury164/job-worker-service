package httptransport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
)

func Routes(h *Handler) http.Handler {
	r := chi.NewRouter()

	// базовые middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// наш логгер (после RequestID)
	r.Use(RequestLogger)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	r.Route("/jobs", func(r chi.Router) {
		r.Post("/", h.CreateJob)
		r.Get("/{id}", h.GetJob)
		r.Get("/{id}/result", h.GetJobResult)
	})

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	return r
}
