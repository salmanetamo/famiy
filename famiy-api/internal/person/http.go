package person

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/httpx"
)

const dateLayout = "2006-01-02"

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes(r chi.Router) {
	r.Post("/persons", h.create)
	r.Get("/persons/{id}", h.get)
}

// createRequest and personResponse are the wire format; they are kept apart
// from the domain type so the API can evolve independently.
type createRequest struct {
	GivenName   string  `json:"given_name"`
	FamilyName  *string `json:"family_name"`
	BirthDate   *string `json:"birth_date"`
	BirthApprox bool    `json:"birth_approx"`
	DeathDate   *string `json:"death_date"`
}

type personResponse struct {
	ID          int64     `json:"id"`
	GivenName   string    `json:"given_name"`
	FamilyName  *string   `json:"family_name"`
	BirthDate   *string   `json:"birth_date"`
	BirthApprox bool      `json:"birth_approx"`
	DeathDate   *string   `json:"death_date"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	birth, err := parseDate("birth_date", req.BirthDate)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	death, err := parseDate("death_date", req.DeathDate)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Create(r.Context(), Input{
		GivenName:   req.GivenName,
		FamilyName:  req.FamilyName,
		BirthDate:   birth,
		BirthApprox: req.BirthApprox,
		DeathDate:   death,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/persons/%d", p.ID))
	httpx.JSON(w, r, http.StatusCreated, toResponse(p))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, toResponse(p))
}

func parseDate(field string, s *string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, *s)
	if err != nil {
		return nil, apperr.Invalid("validation_failed", field+" must be a date in YYYY-MM-DD format")
	}
	return &t, nil
}

func formatDate(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}

func toResponse(p Person) personResponse {
	return personResponse{
		ID:          p.ID,
		GivenName:   p.GivenName,
		FamilyName:  p.FamilyName,
		BirthDate:   formatDate(p.BirthDate),
		BirthApprox: p.BirthApprox,
		DeathDate:   formatDate(p.DeathDate),
		CreatedAt:   p.CreatedAt,
	}
}
