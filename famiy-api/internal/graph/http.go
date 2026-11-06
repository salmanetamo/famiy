package graph

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

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
	r.Get("/graph", h.get)
}

// Wire format, kept apart from the domain types.
type nodeResponse struct {
	ID          int64   `json:"id"`
	GivenName   string  `json:"given_name"`
	FamilyName  *string `json:"family_name"`
	BirthDate   *string `json:"birth_date"`
	BirthApprox bool    `json:"birth_approx"`
	DeathDate   *string `json:"death_date"`
}

type linkResponse struct {
	ParentID int64 `json:"parent_id"`
	ChildID  int64 `json:"child_id"`
}

type partnershipResponse struct {
	Partners [2]int64 `json:"partners"` // unordered pair
	Kind     string   `json:"kind"`
}

type graphResponse struct {
	Persons      []nodeResponse        `json:"persons"`
	ParentLinks  []linkResponse        `json:"parent_links"`
	Partnerships []partnershipResponse `json:"partnerships"`
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
	g, err := h.svc.Get(r.Context(), ownerID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := graphResponse{
		Persons:      make([]nodeResponse, len(g.Persons)),
		ParentLinks:  make([]linkResponse, len(g.ParentLinks)),
		Partnerships: make([]partnershipResponse, len(g.Partnerships)),
	}
	for i, n := range g.Persons {
		out.Persons[i] = nodeResponse{
			ID: n.ID, GivenName: n.GivenName, FamilyName: n.FamilyName,
			BirthDate: formatDate(n.BirthDate), BirthApprox: n.BirthApprox, DeathDate: formatDate(n.DeathDate),
		}
	}
	for i, l := range g.ParentLinks {
		out.ParentLinks[i] = linkResponse(l)
	}
	for i, p := range g.Partnerships {
		out.Partnerships[i] = partnershipResponse{Partners: [2]int64{p.A, p.B}, Kind: p.Kind}
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

func formatDate(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dateLayout)
	return &s
}
