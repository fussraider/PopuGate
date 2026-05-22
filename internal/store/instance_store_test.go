package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestInstanceStore_CountEmpty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)

	count, err := s.Count(context.Background())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestInstanceStore_CreateAndGetByPort(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	inst := &model.Instance{
		Port:        8443,
		MetricsPort: 9091,
		Enabled:     true,
		Label:       "secondary",
	}
	if err := s.Create(ctx, inst); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID == 0 {
		t.Fatal("expected ID to be set after Create")
	}

	got, err := s.GetByPort(ctx, 8443)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if got == nil {
		t.Fatal("expected instance, got nil")
	}
	if got.Port != 8443 {
		t.Fatalf("expected port 8443, got %d", got.Port)
	}
	if got.MetricsPort != 9091 {
		t.Fatalf("expected metrics_port 9091, got %d", got.MetricsPort)
	}
	if got.Label != "secondary" {
		t.Fatalf("expected label 'secondary', got %s", got.Label)
	}
}

func TestInstanceStore_ListMultiple(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	for _, inst := range []model.Instance{
		{Port: 443, MetricsPort: 9090, Enabled: true, Label: "default"},
		{Port: 8443, MetricsPort: 9091, Enabled: true, Label: "secondary"},
		{Port: 9443, MetricsPort: 9092, Enabled: false, Label: "disabled"},
	} {
		if err := s.Create(ctx, &inst); err != nil {
			t.Fatalf("Create port %d: %v", inst.Port, err)
		}
	}

	instances, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(instances) != 3 {
		t.Fatalf("expected 3 instances, got %d", len(instances))
	}

	// Verify ordering by port
	if instances[0].Port != 443 {
		t.Fatalf("expected first port 443, got %d", instances[0].Port)
	}
	if instances[1].Port != 8443 {
		t.Fatalf("expected second port 8443, got %d", instances[1].Port)
	}
	if instances[2].Port != 9443 {
		t.Fatalf("expected third port 9443, got %d", instances[2].Port)
	}
}

func TestInstanceStore_Delete(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Instance{
		Port: 5555, MetricsPort: 9095, Enabled: true, Label: "temp",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, 5555); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := s.GetByPort(ctx, 5555)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestInstanceStore_GetByPortNonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)

	got, err := s.GetByPort(context.Background(), 9999)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent port")
	}
}

func TestInstanceStore_CreateWithTCPMSS(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	inst := &model.Instance{
		Port:          8443,
		MetricsPort:   9091,
		Enabled:       true,
		Label:         "tcpmss",
		TLSDomain:     "example.com",
		TCPMSSEnabled: true,
		TCPMSS:        120,
	}
	if err := s.Create(ctx, inst); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.GetByPort(ctx, 8443)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if !got.TCPMSSEnabled {
		t.Error("expected tcp_mss_enabled=true")
	}
	if got.TCPMSS != 120 {
		t.Errorf("expected tcp_mss=120, got %d", got.TCPMSS)
	}
}

func TestInstanceStore_CreateWithTLSFronting(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	inst := &model.Instance{
		Port:        8443,
		MetricsPort: 9091,
		Enabled:     true,
		Label:       "fronting",
		TLSDomain:   "example.com",
		FakeTLS:     true,
		TLSFronting: true,
	}
	if err := s.Create(ctx, inst); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.GetByPort(ctx, 8443)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if !got.TLSFronting {
		t.Error("expected tls_fronting=true")
	}
}

func TestInstanceStore_UpdateTCPMSS(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	inst := &model.Instance{
		Port:        8443,
		MetricsPort: 9091,
		Enabled:     true,
		Label:       "test",
		TLSDomain:   "example.com",
	}
	if err := s.Create(ctx, inst); err != nil {
		t.Fatalf("Create: %v", err)
	}

	inst.TCPMSSEnabled = true
	inst.TCPMSS = 200
	if err := s.Update(ctx, inst); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.GetByID(ctx, inst.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got.TCPMSSEnabled {
		t.Error("expected tcp_mss_enabled=true after update")
	}
	if got.TCPMSS != 200 {
		t.Errorf("expected tcp_mss=200 after update, got %d", got.TCPMSS)
	}
}

func TestInstanceStore_DeleteByID(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	inst := &model.Instance{
		Port: 7777, MetricsPort: 9077, Enabled: true, Label: "byid",
	}
	if err := s.Create(ctx, inst); err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := inst.ID

	if err := s.DeleteByID(ctx, id); err != nil {
		t.Fatalf("DeleteByID: %v", err)
	}

	got, err := s.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after DeleteByID")
	}
}

func TestInstanceStore_DefaultAntiBlock(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	inst := &model.Instance{
		Port:        8443,
		MetricsPort: 9091,
		Enabled:     true,
		Label:       "defaults",
		TLSDomain:   "example.com",
	}
	_ = inst.Validate()
	if err := s.Create(ctx, inst); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.GetByID(ctx, inst.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.TCPMSSEnabled {
		t.Error("expected tcp_mss_enabled=false by default")
	}
	if got.TCPMSS != 88 {
		t.Errorf("expected tcp_mss=88 by default, got %d", got.TCPMSS)
	}
	if got.TLSFronting {
		t.Error("expected tls_fronting=false by default")
	}
}
