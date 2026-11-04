// Package app is the composition root: it wires modules to the router. It is
// separate from cmd/server so integration tests can build the same handler.
package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/salmanetamo/famiy/famiy-api/internal/identity"
	"github.com/salmanetamo/famiy/famiy-api/internal/person"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/config"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/httpx"
	"github.com/salmanetamo/famiy/famiy-api/internal/relationship"
)

// Config is what the router needs beyond the database pool.
type Config struct {
	Identity       identity.Config
	Handler        identity.HandlerConfig
	AllowedOrigins []string
}

// ConfigFrom derives the router configuration from the environment-backed config.
func ConfigFrom(c config.Config) Config {
	return Config{
		Identity: identity.Config{
			RegistrationEnabled: c.RegistrationEnabled,
			MaxUsers:            c.MaxUsers,
			SessionTTL:          c.SessionTTL,
		},
		Handler:        identity.HandlerConfig{CookieSecure: c.CookieSecure, TrustedProxies: c.TrustedProxies},
		AllowedOrigins: c.AllowedOrigins,
	}
}

// Services exposes the wired services (tests use them to seed data).
type Services struct {
	Persons       *person.Service
	Relationships *relationship.Service
	Identity      *identity.Service
}

// Pinger reports whether the database is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// NewRouter builds the HTTP handler and the services behind it.
func NewRouter(pool *pgxpool.Pool, cfg Config) (http.Handler, Services, error) {
	personSvc := person.NewService(person.NewPgRepository(pool))
	relSvc := relationship.NewService(relationship.NewPgRepository(pool), personSvc)
	identSvc, err := identity.NewService(identity.NewPgRepository(pool), cfg.Identity)
	if err != nil {
		return nil, Services{}, fmt.Errorf("create identity service: %w", err)
	}

	r := chi.NewRouter()
	// Order: request id first so every log line and response has one; the logger
	// wraps the recoverer so recovered panics are logged as 500s.
	r.Use(middleware.RequestID, httpx.RequestLogger, httpx.Recoverer, httpx.SecurityHeaders,
		httpx.OriginCheck(cfg.AllowedOrigins...), httpx.RequireJSON)

	r.Get("/healthz", healthz(pool))
	identHandler := identity.NewHandler(identSvc, cfg.Handler)
	identHandler.Routes(r)

	// Everything below requires a signed-in account and is scoped to it.
	owner := func(ctx context.Context) (int64, bool) {
		u, ok := identity.UserFromContext(ctx)
		return u.ID, ok
	}
	r.Group(func(r chi.Router) {
		r.Use(identHandler.RequireAuth)
		person.NewHandler(personSvc, owner).Routes(r)
		relationship.NewHandler(relSvc, owner).Routes(r)
	})

	return r, Services{Persons: personSvc, Relationships: relSvc, Identity: identSvc}, nil
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
