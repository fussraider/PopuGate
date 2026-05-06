package handler

import (
	"fmt"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/gin-gonic/gin"
)

// SetAuditSvc stores the audit service in the Gin context for use by handlers.
func SetAuditSvc(c *gin.Context, svc *service.AuditService) {
	c.Set("__auditSvc", svc)
}

// HandleError logs the error to Gin context and sends a JSON response.
func HandleError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		c.Error(err) // Record error for GinLogger
		if message != "" {
			c.JSON(status, gin.H{"error": fmt.Sprintf("%s: %v", message, err)})
		} else {
			c.JSON(status, gin.H{"error": err.Error()})
		}
	} else {
		c.JSON(status, gin.H{"error": message})
	}
}

// auditLog records an audit entry using the service from context.
// If no audit service is available, it silently skips.
func auditLog(c *gin.Context, action, detail string) {
	v, _ := c.Get("__auditSvc")
	if v == nil {
		return
	}
	svc, ok := v.(*service.AuditService)
	if !ok || svc == nil {
		return
	}
	user := c.GetString("username")
	if user == "" {
		user = "system"
	}
	_ = svc.Log(c.Request.Context(), user, action, detail)
}
