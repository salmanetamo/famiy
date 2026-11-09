package relationship

import (
	"encoding/json"
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
	r.Post("/persons/{id}/partners", h.addPartner)
	r.Patch("/persons/{id}/partners/{partnerId}", h.updatePartner)
	r.Delete("/persons/{id}/partners/{partnerId}", h.removePartner)
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

// addPartnerRequest and partnerResponse are the wire format of a partnership,
// seen from the person in the path. The storage order of the pair never shows.
type addPartnerRequest struct {
	PartnerID int64           `json:"partner_id"`
	Kind      json.RawMessage `json:"kind"`
}

// parseKind reads an optional "kind" from a request body: absent gives nil,
// null is refused (a partnership always has a kind), anything else must be a
// string. Whether it is an allowed kind is checked by the service.
func parseKind(raw json.RawMessage) (*string, error) {
	if raw == nil { // RawMessage is only set when the key appears
		return nil, nil
	}
	if string(raw) == "null" {
		return nil, apperr.Invalid("validation_failed", "kind must not be null")
	}
	var k string
	if err := json.Unmarshal(raw, &k); err != nil {
		return nil, apperr.Invalid("invalid_json", "field \"kind\" has the wrong type")
	}
	return &k, nil
}

type partnerResponse struct {
	PartnerID int64  `json:"partner_id"`
	Kind      string `json:"kind"`
}

// addPartner records a partnership between the person in the path and another.
// There is no Location header: a partnership has no URL of its own.
func (h *Handler) addPartner(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
	personID, err := httpx.PathID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req addPartnerRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.PartnerID <= 0 {
		httpx.Error(w, r, apperr.Invalid("validation_failed", "partner_id must be a positive integer"))
		return
	}
	kind := DefaultKind
	given, err := parseKind(req.Kind)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if given != nil {
		kind = *given
	}
	p := Partnership{PersonID: personID, PartnerID: req.PartnerID, Kind: kind}
	if err := h.svc.AddPartnership(r.Context(), ownerID, p); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, partnerResponse{PartnerID: p.PartnerID, Kind: p.Kind})
}

// updatePartnerRequest is a JSON Merge Patch: an absent kind is left alone and
// null is refused, because a partnership always has a kind.
type updatePartnerRequest struct {
	Kind json.RawMessage `json:"kind"`
}

func (h *Handler) updatePartner(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
	personID, err := httpx.PathID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	partnerID, err := httpx.PathParamID(r, "partnerId")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req updatePartnerRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	kind, err := parseKind(req.Kind)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.UpdatePartnership(r.Context(), ownerID, personID, partnerID, kind)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, partnerResponse{PartnerID: partnerID, Kind: p.Kind})
}

// removePartner deletes the partnership between the person in the path and one
// of their partners.
func (h *Handler) removePartner(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := httpx.RequireOwner(w, r, h.owner)
	if !ok {
		return
	}
	personID, err := httpx.PathID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	partnerID, err := httpx.PathParamID(r, "partnerId")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.RemovePartnership(r.Context(), ownerID, personID, partnerID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
