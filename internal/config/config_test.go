package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeServiceType(t *testing.T) {
	tests := []struct {
		name     string
		service  string
		fallback string
		expected string
	}{
		{"Empty with fallback", "", "cups", "_cups._tcp"},
		{"Standard DNS-SD", "_immich._tcp", "", "_immich._tcp"},
		{"Bare name without prefix or suffix", "ipp", "", "_ipp._tcp"},
		{"Bare name with underscore but no proto", "_hap", "", "_hap._tcp"},
		{"UDP service", "_printer._udp", "", "_printer._udp"},
		{"Whitespace trimming", "  _media._tcp  ", "", "_media._tcp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DiscoveryConfig{Service: tt.service}
			actual := d.NormalizeServiceType(tt.fallback)
			if actual != tt.expected {
				t.Errorf("NormalizeServiceType() = %q, expected %q", actual, tt.expected)
			}
		})
	}
}

func TestFormattedTXT(t *testing.T) {
	tests := []struct {
		name     string
		txt      map[string]string
		expected []string
	}{
		{"Empty TXT defaults to path=/", nil, []string{"path=/"}},
		{"Empty map defaults to path=/", map[string]string{}, []string{"path=/"}},
		{
			"Key-value pairs are sorted",
			map[string]string{"version": "1.0", "path": "/api"},
			[]string{"path=/api", "version=1.0"},
		},
		{
			"Boolean flag without value",
			map[string]string{"secure": ""},
			[]string{"secure"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DiscoveryConfig{TXT: tt.txt}
			actual := d.FormattedTXT()
			if !reflect.DeepEqual(actual, tt.expected) {
				t.Errorf("FormattedTXT() = %v, expected %v", actual, tt.expected)
			}
		})
	}
}

func TestConfigJSONUnmarshaling(t *testing.T) {
	rawJSON := `{
		"advertise_addr": "auto",
		"services": [
			{
				"name": "immich",
				"local_port": 2283,
				"upstream_url": "http://photos:2283",
				"discovery": {
					"type": "immich",
					"service": "_immich._tcp",
					"txt": {
						"path": "/api"
					}
				}
			},
			{
				"name": "office-printer",
				"local_port": 631,
				"upstream_url": "http://cups:631",
				"discovery": {
					"type": "mdns",
					"service": "_ipp._tcp",
					"txt": {
						"rp": "printers/LaserJet",
						"pdl": "application/pdf"
					}
				}
			}
		]
	}`

	var cfg Config
	if err := json.Unmarshal([]byte(rawJSON), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal config JSON: %v", err)
	}

	if len(cfg.Services) != 2 {
		t.Fatalf("Expected 2 services, got %d", len(cfg.Services))
	}

	immich := cfg.Services[0]
	if immich.Discovery.Type != "immich" || immich.Discovery.Service != "_immich._tcp" {
		t.Errorf("Unexpected immich config: %+v", immich.Discovery)
	}

	printer := cfg.Services[1]
	if printer.Discovery.Type != "mdns" || printer.Discovery.Service != "_ipp._tcp" {
		t.Errorf("Unexpected printer config: %+v", printer.Discovery)
	}
}

func TestDefaultConfigWithImmich(t *testing.T) {
	cfg := DefaultConfig("auto", "http://jellyfin:8096", "http://plex:32400", "http://immich:2283")
	if len(cfg.Services) != 3 {
		t.Fatalf("Expected 3 services in DefaultConfig, got %d", len(cfg.Services))
	}

	immichSvc := cfg.Services[2]
	if immichSvc.Name != "immich" || immichSvc.LocalPort != 2283 {
		t.Errorf("Unexpected immich default service: %+v", immichSvc)
	}
	if immichSvc.Discovery.Type != "immich" || immichSvc.Discovery.Service != "_immich._tcp" {
		t.Errorf("Unexpected immich discovery: %+v", immichSvc.Discovery)
	}
}
