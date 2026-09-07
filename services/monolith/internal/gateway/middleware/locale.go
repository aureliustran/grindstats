package middleware

import (
	"github.com/gin-gonic/gin"

	"grindstats/libs/httpkit"
	"grindstats/libs/i18n"
)

// Locale resolves Accept-Language once per request, stores it under
// httpkit.LocaleContextKey so httpkit.Error need not re-parse the header,
// and sets Content-Language on the response — the three-planes rule
// (docs/audit-and-errors.md §1a) made visible to the caller: the locale
// affects only what is rendered back, never storage.
func Locale() gin.HandlerFunc {
	return func(c *gin.Context) {
		resolved := i18n.ResolveLocale(c.GetHeader("Accept-Language"))
		c.Set(httpkit.LocaleContextKey, resolved)
		c.Writer.Header().Set("Content-Language", resolved)
		c.Next()
	}
}
