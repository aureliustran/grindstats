package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS allows only the configured origins, reflecting the request's Origin
// back (rather than "*") because the auth stories will need credentialed
// requests, which "*" cannot carry. An origin not on the list gets no
// Access-Control-Allow-Origin header at all, which the browser treats as a
// same-origin-only response.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Vary", "Origin")
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}

		if c.Request.Method == http.MethodOptions {
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept-Language, X-Request-ID, X-CSRF-Token")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
