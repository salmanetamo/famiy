package person

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/httpx"
)

const dateLayout = "2006-01-02"

type Handler struct {
	svc   *Service
	owner httpx.OwnerFunc
}

func NewHandler(svc *Service, owner httpx.OwnerFunc) *Handler {
	return &Handler{svc: svc, owner: owner}
}

func (h *Handler) Routes(r chi.Router) {
	r.Post("/persons", h.create)
	r.Get("/persons", h.search)
	r.Get("/persons/{id}", h.get)
	r.Patch("/persons/{id}", h.patch)
	r.Delete("/persons/{id}", h.delete)
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
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
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
	p, err := h.svc.Create(r.Context(), ownerID, Input{
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
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
	id, err := httpx.PathID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Get(r.Context(), ownerID, id)
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

// summaryResponse is a person as search returns it: the same fields the graph
// read model carries, without created_at.
type summaryResponse struct {
	ID          int64   `json:"id"`
	GivenName   string  `json:"given_name"`
	FamilyName  *string `json:"family_name"`
	BirthDate   *string `json:"birth_date"`
	BirthApprox bool    `json:"birth_approx"`
	DeathDate   *string `json:"death_date"`
}

type searchResponse struct {
	Persons []summaryResponse `json:"persons"`
	HasMore bool              `json:"has_more"`
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
	limit := DefaultSearchLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httpx.Error(w, r, apperr.Invalid("validation_failed", "limit must be an integer"))
			return
		}
		limit = n
	}
	res, err := h.svc.Search(r.Context(), ownerID, r.URL.Query().Get("q"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := searchResponse{Persons: make([]summaryResponse, len(res.Persons)), HasMore: res.HasMore}
	for i, p := range res.Persons {
		out.Persons[i] = summaryResponse{
			ID: p.ID, GivenName: p.GivenName, FamilyName: p.FamilyName,
			BirthDate: formatDate(p.BirthDate), BirthApprox: p.BirthApprox, DeathDate: formatDate(p.DeathDate),
		}
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// field records whether a JSON key was present and whether its value was null,
// which encoding/json cannot tell apart on a plain pointer.
type field[T any] struct {
	Present bool
	Null    bool
	Value   T
}

func (f *field[T]) UnmarshalJSON(b []byte) error {
	// Start clean: with a duplicate key the last one must win, not mix with the first.
	*f = field[T]{Present: true}
	if string(b) == "null" {
		f.Null = true
		return nil
	}
	return json.Unmarshal(b, &f.Value)
}

// patchRequest is a JSON Merge Patch body: an absent key leaves the field
// alone, null clears it (where it can be cleared) and a value replaces it.
type patchRequest struct {
	GivenName   field[string] `json:"given_name"`
	FamilyName  field[string] `json:"family_name"`
	BirthDate   field[string] `json:"birth_date"`
	BirthApprox field[bool]   `json:"birth_approx"`
	DeathDate   field[string] `json:"death_date"`
}

func (req patchRequest) toPatch() (Patch, error) {
	var p Patch
	if req.GivenName.Null {
		return Patch{}, apperr.Invalid("validation_failed", "given_name must not be null")
	}
	if req.BirthApprox.Null {
		return Patch{}, apperr.Invalid("validation_failed", "birth_approx must not be null")
	}
	if req.GivenName.Present {
		p.GivenName = &req.GivenName.Value
	}
	if req.BirthApprox.Present {
		p.BirthApprox = &req.BirthApprox.Value
	}
	if req.FamilyName.Present {
		p.FamilyName.Set = true
		if !req.FamilyName.Null {
			p.FamilyName.Value = &req.FamilyName.Value
		}
	}
	for _, d := range []struct {
		name string
		in   field[string]
		out  *Nullable[time.Time]
	}{
		{"birth_date", req.BirthDate, &p.BirthDate},
		{"death_date", req.DeathDate, &p.DeathDate},
	} {
		if !d.in.Present {
			continue
		}
		d.out.Set = true
		if d.in.Null {
			continue
		}
		t, err := parseDate(d.name, &d.in.Value)
		if err != nil {
			return Patch{}, err
		}
		d.out.Value = t
	}
	return p, nil
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
	id, err := httpx.PathID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req patchRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	patch, err := req.toPatch()
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Update(r.Context(), ownerID, id, patch)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, toResponse(p))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
	id, err := httpx.PathID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), ownerID, id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
