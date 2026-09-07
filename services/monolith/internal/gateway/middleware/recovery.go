package middleware

import (
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"grindstats/libs/httpkit"
)

// Recovery catches a panic in any downstream handler, logs it with the
// request ID and stack trace, and writes the INTERNAL_ERROR envelope instead
// of letting Gin close the connection with no body. It must be the first
// middleware registered so every later stage's panics are caught too.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					"request_id", RequestIDFrom(c),
					"path", c.Request.URL.Path,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				httpkit.InternalError(c)
			}
		}()
		c.Next()
	}
}
