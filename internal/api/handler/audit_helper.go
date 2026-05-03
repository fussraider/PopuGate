package handler

import (
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/gin-gonic/gin"
)

// SetAuditSvc stores the audit service in the Gin context for use by handlers.
func SetAuditSvc(c *gin.Context, svc *service.AuditService) {
	c.Set("__auditSvc", svc)
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
