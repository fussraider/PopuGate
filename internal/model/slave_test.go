package model

import "testing"

func TestSlaveValidate(t *testing.T) {
	tests := []struct {
		name      string
		slave     Slave
		wantErr   bool
		wantLabel string // expected label after Validate
	}{
		{
			name: "valid with host and port",
			slave: Slave{
				Host:  "192.168.1.100",
				Port:  22,
				Label: "worker-1",
			},
			wantErr:   false,
			wantLabel: "worker-1",
		},
		{
			name: "valid label defaults to host",
			slave: Slave{
				Host:  "10.0.0.5",
				Port:  22,
				Label: "",
			},
			wantErr:   false,
			wantLabel: "10.0.0.5",
		},
		{
			name: "empty host invalid",
			slave: Slave{
				Host:  "",
				Port:  22,
				Label: "test",
			},
			wantErr: true,
		},
		{
			name: "port zero invalid",
			slave: Slave{
				Host:  "192.168.1.100",
				Port:  0,
				Label: "test",
			},
			wantErr: true,
		},
		{
			name: "port 65536 invalid",
			slave: Slave{
				Host:  "192.168.1.100",
				Port:  65536,
				Label: "test",
			},
			wantErr: true,
		},
		{
			name: "negative port invalid",
			slave: Slave{
				Host:  "192.168.1.100",
				Port:  -1,
				Label: "test",
			},
			wantErr: true,
		},
		{
			name: "port 1 valid",
			slave: Slave{
				Host:  "192.168.1.100",
				Port:  1,
				Label: "test",
			},
			wantErr:   false,
			wantLabel: "test",
		},
		{
			name: "port 65535 valid",
			slave: Slave{
				Host:  "192.168.1.100",
				Port:  65535,
				Label: "test",
			},
			wantErr:   false,
			wantLabel: "test",
		},
		{
			name: "hostname with domain",
			slave: Slave{
				Host:  "slave.example.com",
				Port:  2222,
				Label: "",
			},
			wantErr:   false,
			wantLabel: "slave.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.slave.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.slave.Label != tt.wantLabel {
				t.Errorf("Validate() Label = %q, want %q", tt.slave.Label, tt.wantLabel)
			}
		})
	}
}

func TestSlaveValidateLabelDefaultDoesNotOverride(t *testing.T) {
	// When Label is already set, Validate should not override it
	s := Slave{
		Host:  "10.0.0.1",
		Port:  22,
		Label: "custom-label",
	}
	err := s.Validate()
	if err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
	if s.Label != "custom-label" {
		t.Errorf("Label = %q, want %q (should not be overridden)", s.Label, "custom-label")
	}
}

func TestSlaveValidateBothEmptyHostAndLabel(t *testing.T) {
	s := Slave{
		Host:  "",
		Port:  22,
		Label: "",
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for empty host")
	}
}

func TestSlaveValidateEmptyHostNonEmptyLabel(t *testing.T) {
	// Label defaulting only happens if host is valid; empty host should error first
	s := Slave{
		Host:  "",
		Port:  22,
		Label: "some-label",
	}
	err := s.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for empty host even when label is set")
	}
}
