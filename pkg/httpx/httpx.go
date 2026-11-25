// Package httpx has shared Gin middleware and response helpers.
package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDHeader = "X-Request-ID"

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Header(RequestIDHeader, id)
		c.Request.Header.Set(RequestIDHeader, id)
		c.Next()
	}
}

func AccessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("request", "method", c.Request.Method, "path", c.Request.URL.Path,
			"status", c.Writer.Status(), "ms", time.Since(start).Milliseconds(), "request_id", c.GetString("request_id"))
	}
}

type ErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func Fail(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, ErrorBody{Error: msg, Code: code})
}

func Health(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) }
