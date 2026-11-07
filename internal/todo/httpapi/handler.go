// Package httpapi exposes the todo use cases over HTTP with gin.
package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

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

// Register mounts the routes on r, which must already authenticate the caller.
func (h *Handler) Register(r gin.IRouter) {
	lists := r.Group("/lists")
	{
		lists.POST("/", h.createList)
		lists.GET("/", h.getAllLists)
		lists.GET("/:id", h.getListByID)
		lists.PUT("/:id", h.updateList)
		lists.DELETE("/:id", h.deleteList)

		items := lists.Group(":id/items")
		{
			items.POST("/", h.createItem)
			items.GET("/", h.getAllItems)
		}
	}

	items := r.Group("/items")
	{
		items.GET("/:id", h.getItemByID)
		items.PUT("/:id", h.updateItem)
		items.DELETE("/:id", h.deleteItem)
	}
}

func (h *Handler) createList(c *gin.Context) {
	userID, ok := httpx.UserID(c)
	if !ok {
		return
	}
	var req createRequest
	if !httpx.BindJSON(c, &req) {
		return
	}

	id, err := h.svc.CreateList(c.Request.Context(), userID, req.Title, req.Description)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpx.IDResponse{ID: id})
}

func (h *Handler) getAllLists(c *gin.Context) {
	userID, ok := httpx.UserID(c)
	if !ok {
		return
	}

	lists, err := h.svc.Lists(c.Request.Context(), userID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, listsResponse{Data: toListResponses(lists)})
}

func (h *Handler) getListByID(c *gin.Context) {
	userID, ok := httpx.UserID(c)
	if !ok {
		return
	}
	listID, ok := httpx.PathID(c)
	if !ok {
		return
	}

	list, err := h.svc.List(c.Request.Context(), userID, listID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toListResponse(list))
}

func (h *Handler) updateList(c *gin.Context) {
	userID, ok := httpx.UserID(c)
	if !ok {
		return
	}
	listID, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req updateListRequest
	if !httpx.BindJSON(c, &req) {
		return
	}

	patch := domain.ListPatch{Title: req.Title, Description: req.Description}
	if err := h.svc.UpdateList(c.Request.Context(), userID, listID, patch); err != nil {
		writeError(c, err)
		return
	}
	httpx.OK(c)
}

func (h *Handler) deleteList(c *gin.Context) {
	userID, ok := httpx.UserID(c)
	if !ok {
		return
	}
	listID, ok := httpx.PathID(c)
	if !ok {
		return
	}

	if err := h.svc.DeleteList(c.Request.Context(), userID, listID); err != nil {
		writeError(c, err)
		return
	}
	httpx.OK(c)
}

func (h *Handler) createItem(c *gin.Context) {
	userID, ok := httpx.UserID(c)
	if !ok {
		return
	}
	listID, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req createRequest
	if !httpx.BindJSON(c, &req) {
		return
	}

	id, err := h.svc.AddItem(c.Request.Context(), userID, listID, req.Title, req.Description)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, httpx.IDResponse{ID: id})
}

func (h *Handler) getAllItems(c *gin.Context) {
	userID, ok := httpx.UserID(c)
	if !ok {
		return
	}
	listID, ok := httpx.PathID(c)
	if !ok {
		return
	}

	items, err := h.svc.Items(c.Request.Context(), userID, listID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toItemResponses(items))
}

func (h *Handler) getItemByID(c *gin.Context) {
	userID, ok := httpx.UserID(c)
	if !ok {
		return
	}
	itemID, ok := httpx.PathID(c)
	if !ok {
		return
	}

	item, err := h.svc.Item(c.Request.Context(), userID, itemID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, toItemResponse(item))
}

func (h *Handler) updateItem(c *gin.Context) {
	userID, ok := httpx.UserID(c)
	if !ok {
		return
	}
	itemID, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req updateItemRequest
	if !httpx.BindJSON(c, &req) {
		return
	}

	patch := domain.ItemPatch{Title: req.Title, Description: req.Description, Done: req.Done}
	if err := h.svc.UpdateItem(c.Request.Context(), userID, itemID, patch); err != nil {
		writeError(c, err)
		return
	}
	httpx.OK(c)
}

func (h *Handler) deleteItem(c *gin.Context) {
	userID, ok := httpx.UserID(c)
	if !ok {
		return
	}
	itemID, ok := httpx.PathID(c)
	if !ok {
		return
	}

	if err := h.svc.DeleteItem(c.Request.Context(), userID, itemID); err != nil {
		writeError(c, err)
		return
	}
	httpx.OK(c)
}

// writeError maps domain errors to HTTP statuses.
func writeError(c *gin.Context, err error) {
	var invalid *domain.ValidationError
	switch {
	case errors.Is(err, domain.ErrOwnerNotFound):
		httpx.Error(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrListNotFound), errors.Is(err, domain.ErrItemNotFound):
		httpx.Error(c, http.StatusNotFound, err.Error())
	case errors.As(err, &invalid):
		httpx.Error(c, http.StatusBadRequest, err.Error())
	default:
		httpx.InternalError(c, err)
	}
}
