package service

import (
	"context"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
)

// AuditService handles audit log business logic.
type AuditService struct {
	audit *store.AuditStore
}

// NewAuditService creates a new AuditService.
func NewAuditService(audit *store.AuditStore) *AuditService {
	return &AuditService{audit: audit}
}

// Log records an audit entry.
func (s *AuditService) Log(ctx context.Context, user, action, detail string) error {
	return s.audit.Insert(ctx, user, action, detail)
}

// List returns audit entries with pagination and optional filters.
func (s *AuditService) List(ctx context.Context, limit, offset int, filter *model.AuditFilter) ([]model.AuditEntry, error) {
	return s.audit.List(ctx, limit, offset, filter)
}

// CleanOld removes audit entries older than 30 days.
func (s *AuditService) CleanOld(ctx context.Context) (int, error) {
	return s.audit.CleanOld(ctx, 30*24*time.Hour)
}

// GetFilters returns unique users and actions from the audit store.
func (s *AuditService) GetFilters(ctx context.Context) ([]string, []string, error) {
	return s.audit.GetFilters(ctx)
}
