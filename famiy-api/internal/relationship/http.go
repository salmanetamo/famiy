package relationship

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/salmanetamo/famiy/famiy-api/internal/platform/apperr"
	"github.com/salmanetamo/famiy/famiy-api/internal/platform/httpx"
)

type Handler struct {
	svc   *Service
	owner httpx.OwnerFunc
}

func NewHandler(svc *Service, owner httpx.OwnerFunc) *Handler {
	return &Handler{svc: svc, owner: owner}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/persons/{id}/ancestors", h.ancestors)
	r.Post("/persons/{id}/parents", h.addParent)
	r.Delete("/persons/{id}/parents/{parentId}", h.removeParent)
}

// addParentRequest and parentLinkResponse are the wire format of a parent link.
type addParentRequest struct {
	ParentID int64 `json:"parent_id"`
}

type parentLinkResponse struct {
	ChildID  int64 `json:"child_id"`
	ParentID int64 `json:"parent_id"`
}

// addParent links the person in the path (the child) to a parent. There is no
// Location header: a link has no URL of its own.
func (h *Handler) addParent(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
	childID, err := httpx.PathID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req addParentRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.ParentID <= 0 {
		httpx.Error(w, r, apperr.Invalid("validation_failed", "parent_id must be a positive integer"))
		return
	}
	link := ParentLink{ParentID: req.ParentID, ChildID: childID}
	if err := h.svc.AddParentLink(r.Context(), ownerID, link); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, parentLinkResponse{ChildID: childID, ParentID: req.ParentID})
}

// removeParent deletes the link between the person in the path and one of their
// parents. 204 with no body on success.
func (h *Handler) removeParent(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
	childID, err := httpx.PathID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	parentID, err := httpx.PathParamID(r, "parentId")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.RemoveParentLink(r.Context(), ownerID, ParentLink{ParentID: parentID, ChildID: childID}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
	id, err := httpx.PathID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	as, err := h.svc.Ancestors(r.Context(), ownerID, id)
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
