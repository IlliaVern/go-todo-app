// Package httpx holds HTTP helpers shared by the bounded contexts' gin
// adapters: the error and status envelopes, request binding, path ids and the
// authenticated caller's user id.
package httpx

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type errorResponse struct {
	Message string `json:"message"`
}

type statusResponse struct {
	Status string `json:"status"`
}

// IDResponse is returned by endpoints that create a resource.
type IDResponse struct {
	ID int64 `json:"id"`
}

// OK writes the {"status": "ok"} envelope.
func OK(c *gin.Context) {
	c.JSON(http.StatusOK, statusResponse{Status: "ok"})
}

// Error aborts the request with a client-visible message.
func Error(c *gin.Context, status int, message string) {
	slog.InfoContext(c.Request.Context(), "request rejected",
		"status", status, "method", c.Request.Method, "route", c.FullPath(), "reason", message)
	c.AbortWithStatusJSON(status, errorResponse{Message: message})
}

// InternalError logs err and aborts with a generic 500, keeping internals out of the response.
func InternalError(c *gin.Context, err error) {
	slog.ErrorContext(c.Request.Context(), "request failed",
		"method", c.Request.Method, "route", c.FullPath(), "error", err)
	c.AbortWithStatusJSON(http.StatusInternalServerError, errorResponse{Message: "internal server error"})
}

// BindJSON decodes the body into dst, answering 400 on malformed JSON.
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		Error(c, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

// PathID parses the positive integer path parameter ":id", answering 400 otherwise.
func PathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		Error(c, http.StatusBadRequest, "invalid id param")
		return 0, false
	}
	return id, true
}

const userIDKey = "userID"

// SetUserID stores the authenticated caller's id on the request.
func SetUserID(c *gin.Context, id int64) {
	c.Set(userIDKey, id)
}

// UserID returns the authenticated caller's id, answering 500 when the
// authentication middleware did not run.
func UserID(c *gin.Context) (int64, bool) {
	id, ok := c.Get(userIDKey)
	if userID, isInt := id.(int64); ok && isInt {
		return userID, true
	}
	slog.ErrorContext(c.Request.Context(), "user id missing from request context", "route", c.FullPath())
	c.AbortWithStatusJSON(http.StatusInternalServerError, errorResponse{Message: "internal server error"})
	return 0, false
}
