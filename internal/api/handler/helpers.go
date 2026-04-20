package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// respondError writes a JSON error response.
func respondError(c *gin.Context, code int, err error) {
	c.JSON(code, gin.H{"error": err.Error()})
}

// respondOK writes a JSON success response.
func respondOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, data)
}
