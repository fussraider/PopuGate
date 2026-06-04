package service

import (
	"testing"
)

func TestGeoblockService_IsAvailable(t *testing.T) {
	// Verify it runs without crashing.
	svc := &GeoblockService{}
	_, _ = svc.IsAvailable()
}
