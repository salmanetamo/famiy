package relationship

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/persons/{id}/ancestors", h.ancestors)
}

type ancestorResponse struct {
	PersonID    int64 `json:"person_id"`
	Generations int   `json:"generations"`
}

type ancestorsResponse struct {
	PersonID  int64              `json:"person_id"`
	Ancestors []ancestorResponse `json:"ancestors"`
}

func (h *Handler) ancestors(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	as, err := h.svc.Ancestors(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	resp := ancestorsResponse{PersonID: id, Ancestors: make([]ancestorResponse, 0, len(as))}
	for _, a := range as {
		resp.Ancestors = append(resp.Ancestors, ancestorResponse(a))
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}
