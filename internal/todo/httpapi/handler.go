// Package httpapi exposes the todo use cases over net/http.
package httpapi

import (
	"errors"
	"net/http"

	"github.com/IlliaVern/go-todo-app/internal/platform/httpx"
	"github.com/IlliaVern/go-todo-app/internal/todo/app"
	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
)

type Handler struct {
	svc *app.Service
}

func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts the routes on mux behind auth, which must authenticate the
// caller. Collection routes answer with and without a trailing slash.
func (h *Handler) Register(mux *http.ServeMux, auth func(http.Handler) http.Handler) {
	handle := func(pattern string, fn http.HandlerFunc) { mux.Handle(pattern, auth(fn)) }
	collection := func(method, path string, fn http.HandlerFunc) {
		handle(method+" "+path, fn)
		handle(method+" "+path+"/{$}", fn)
	}

	collection(http.MethodPost, "/api/lists", h.createList)
	collection(http.MethodGet, "/api/lists", h.getAllLists)
	handle("GET /api/lists/{id}", h.getListByID)
	handle("PUT /api/lists/{id}", h.updateList)
	handle("DELETE /api/lists/{id}", h.deleteList)

	collection(http.MethodPost, "/api/lists/{id}/items", h.createItem)
	collection(http.MethodGet, "/api/lists/{id}/items", h.getAllItems)

	handle("GET /api/items/{id}", h.getItemByID)
	handle("PUT /api/items/{id}", h.updateItem)
	handle("DELETE /api/items/{id}", h.deleteItem)
}

func (h *Handler) createList(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	var req createRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	id, err := h.svc.CreateList(r.Context(), userID, req.Title, req.Description)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, httpx.IDResponse{ID: id})
}

func (h *Handler) getAllLists(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}

	lists, err := h.svc.Lists(r.Context(), userID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, listsResponse{Data: toListResponses(lists)})
}

func (h *Handler) getListByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	listID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}

	list, err := h.svc.List(r.Context(), userID, listID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, toListResponse(list))
}

func (h *Handler) updateList(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	listID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var req updateListRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	patch := domain.ListPatch{Title: req.Title, Description: req.Description}
	if err := h.svc.UpdateList(r.Context(), userID, listID, patch); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.OK(w, r)
}

func (h *Handler) deleteList(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	listID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}

	if err := h.svc.DeleteList(r.Context(), userID, listID); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.OK(w, r)
}

func (h *Handler) createItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	listID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var req createRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	id, err := h.svc.AddItem(r.Context(), userID, listID, req.Title, req.Description)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, httpx.IDResponse{ID: id})
}

func (h *Handler) getAllItems(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	listID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}

	items, err := h.svc.Items(r.Context(), userID, listID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, toItemResponses(items))
}

func (h *Handler) getItemByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	itemID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}

	item, err := h.svc.Item(r.Context(), userID, itemID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, toItemResponse(item))
}

func (h *Handler) updateItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	itemID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var req updateItemRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}

	patch := domain.ItemPatch{Title: req.Title, Description: req.Description, Done: req.Done}
	if err := h.svc.UpdateItem(r.Context(), userID, itemID, patch); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.OK(w, r)
}

func (h *Handler) deleteItem(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.UserID(w, r)
	if !ok {
		return
	}
	itemID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}

	if err := h.svc.DeleteItem(r.Context(), userID, itemID); err != nil {
		writeError(w, r, err)
		return
	}
	httpx.OK(w, r)
}

// writeError maps domain errors to HTTP statuses.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *domain.ValidationError
	switch {
	case errors.Is(err, domain.ErrOwnerNotFound):
		httpx.Error(w, r, http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrConcurrentUpdate):
		httpx.Error(w, r, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrListNotFound), errors.Is(err, domain.ErrItemNotFound):
		httpx.Error(w, r, http.StatusNotFound, err.Error())
	case errors.As(err, &invalid):
		httpx.Error(w, r, http.StatusBadRequest, err.Error())
	default:
		httpx.InternalError(w, r, err)
	}
}
