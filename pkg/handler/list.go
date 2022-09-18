package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) createList(c *gin.Context) {
	id, _ := c.Get(userCtx)
	c.JSON(http.StatusOK, map[string]interface{}{
		"id": id,
	})
}

func (handler *Handler) getAllLists(c *gin.Context) {

}

func (handler *Handler) getListById(c *gin.Context) {

}

func (handler *Handler) updateList(c *gin.Context) {

}

func (handler *Handler) deleteList(c *gin.Context) {

}
