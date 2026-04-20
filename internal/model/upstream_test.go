package model

import (
	"strings"
	"testing"
)

func TestUpstreamValidate(t *testing.T) {
	tests := []struct {
		name    string
		upstream Upstream
		wantErr bool
	}{
		{
			name: "valid direct upstream",
			upstream: Upstream{
				Name:   "my-direct",
				Type:   UpstreamDirect,
				Weight: 50,
			},
			wantErr: false,
		},
		{
			name: "valid socks5 upstream with address",
			upstream: Upstream{
				Name:    "my-socks5",
				Type:    UpstreamSOCKS5,
				Address: "socks5.example.com:1080",
				Weight:  50,
			},
			wantErr: false,
		},
		{
			name: "valid socks4 upstream with address",
			upstream: Upstream{
				Name:    "my-socks4",
				Type:    UpstreamSOCKS4,
				Address: "socks4.example.com:1080",
				Weight:  50,
			},
			wantErr: false,
		},
		{
			name: "direct with address is valid",
			upstream: Upstream{
				Name:    "direct-addr",
				Type:    UpstreamDirect,
				Address: "unused.example.com",
				Weight:  50,
			},
			wantErr: false,
		},
		{
			name: "invalid upstream type",
			upstream: Upstream{
				Name:   "bad-type",
				Type:   UpstreamType("http"),
				Weight: 50,
			},
			wantErr: true,
		},
		{
			name: "empty name",
			upstream: Upstream{
				Name:   "",
				Type:   UpstreamDirect,
				Weight: 50,
			},
			wantErr: true,
		},
		{
			name: "name too long (33 chars)",
			upstream: Upstream{
				Name:   strings.Repeat("a", 33),
				Type:   UpstreamDirect,
				Weight: 50,
			},
			wantErr: true,
		},
		{
			name: "name exactly 32 chars",
			upstream: Upstream{
				Name:   strings.Repeat("a", 32),
				Type:   UpstreamDirect,
				Weight: 50,
			},
			wantErr: false,
		},
		{
			name: "socks5 without address",
			upstream: Upstream{
				Name:    "no-addr",
				Type:    UpstreamSOCKS5,
				Address: "",
				Weight:  50,
			},
			wantErr: true,
		},
		{
			name: "socks4 without address",
			upstream: Upstream{
				Name:    "no-addr",
				Type:    UpstreamSOCKS4,
				Address: "",
				Weight:  50,
			},
			wantErr: true,
		},
		{
			name: "weight zero",
			upstream: Upstream{
				Name:   "zero-weight",
				Type:   UpstreamDirect,
				Weight: 0,
			},
			wantErr: true,
		},
		{
			name: "weight negative",
			upstream: Upstream{
				Name:   "neg-weight",
				Type:   UpstreamDirect,
				Weight: -5,
			},
			wantErr: true,
		},
		{
			name: "weight 101",
			upstream: Upstream{
				Name:   "high-weight",
				Type:   UpstreamDirect,
				Weight: 101,
			},
			wantErr: true,
		},
		{
			name: "weight 1 valid",
			upstream: Upstream{
				Name:   "min-weight",
				Type:   UpstreamDirect,
				Weight: 1,
			},
			wantErr: false,
		},
		{
			name: "weight 100 valid",
			upstream: Upstream{
				Name:   "max-weight",
				Type:   UpstreamDirect,
				Weight: 100,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.upstream.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
