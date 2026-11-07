package handler

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

type errorResponse struct {
	Message string `json:"message"`
}

type statusResponse struct {
	Status string `json:"status"`
}

func newErrorResponse(ctx *gin.Context, statusCode int, message string) {
	slog.Error("request failed", "status", statusCode, "method", ctx.Request.Method, "path", ctx.FullPath(), "error", message)
	ctx.AbortWithStatusJSON(statusCode, errorResponse{message})
}
