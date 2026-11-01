// Package app is the composition root: it wires modules to the router. It is
// separate from cmd/server so integration tests can build the same handler.
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salmanetamo/famiy/famiy-api/internal/person"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/httpx"
	"github.com/salmanetamo/famiy/famiy-api/internal/relationship"
)

// Services exposes the wired services (tests use them to seed data).
type Services struct {
	Persons       *person.Service
	Relationships *relationship.Service
}

// Pinger reports whether the database is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// NewRouter builds the HTTP handler and the services behind it.
func NewRouter(pool *pgxpool.Pool) (http.Handler, Services) {
	personSvc := person.NewService(person.NewPgRepository(pool))
	relSvc := relationship.NewService(relationship.NewPgRepository(pool), personSvc)

	r := chi.NewRouter()
	r.Use(middleware.RequestID, httpx.RequestLogger, httpx.Recoverer)

	r.Get("/healthz", healthz(pool))
	person.NewHandler(personSvc).Routes(r)
	relationship.NewHandler(relSvc).Routes(r)

	return r, Services{Persons: personSvc, Relationships: relSvc}
}

func healthz(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			httpx.JSON(w, r, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "db": "down"})
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]string{"status": "ok", "db": "up"})
	}
}
