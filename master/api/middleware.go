package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func APIKeyMiddleware() gin.HandlerFunc {
	expectedKey := os.Getenv("API_KEY")
	if expectedKey == "" {
		expectedKey = "distdb-default-key"
	}
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Allow public access to dashboard and health without API key
		if path == "/dashboard" || path == "/health" || strings.HasPrefix(path, "/static") {
			c.Next()
			return
		}

		key := c.GetHeader("X-API-Key")
		if key != expectedKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			return
		}
		c.Next()
	}
}