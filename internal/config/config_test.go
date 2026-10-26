package config

import (
	"encoding/json"
	"os"
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

func TestIsConfigured(t *testing.T) {
	// Empty services
	emptyCfg := &Config{}
	if emptyCfg.IsConfigured() {
		t.Errorf("Empty config should not be considered configured")
	}

	// Template placeholder services
	placeholderCfg := &Config{
		Services: []ServiceConfig{
			{Name: "jellyfin", LocalPort: 8096, UpstreamURL: "http://media-server:8096"},
			{Name: "plex", LocalPort: 32400, UpstreamURL: "http://media-server.your-tailnet.ts.net:32400"},
		},
	}
	if placeholderCfg.IsConfigured() {
		t.Errorf("Placeholder config should not be considered configured")
	}

	// Valid live service
	validCfg := &Config{
		Services: []ServiceConfig{
			{Name: "jellyfin", LocalPort: 8096, UpstreamURL: "http://100.103.61.68:8096"},
		},
	}
	if !validCfg.IsConfigured() {
		t.Errorf("Valid upstream URL should be considered configured")
	}
}

func TestValidateConfig(t *testing.T) {
	valid := &Config{
		AdminPort: 7350,
		Services: []ServiceConfig{
			{Name: "jellyfin", LocalPort: 8096, UpstreamURL: "http://100.64.0.1:8096"},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Expected valid config to pass validation, got: %v", err)
	}

	// Port collision with AdminPort
	portCollision := &Config{
		AdminPort: 7350,
		Services: []ServiceConfig{
			{Name: "bad-service", LocalPort: 7350, UpstreamURL: "http://100.64.0.1:7350"},
		},
	}
	if err := portCollision.Validate(); err == nil {
		t.Errorf("Expected port collision error between service and admin_port")
	}

	// Invalid upstream URL format
	badURL := &Config{
		AdminPort: 7350,
		Services: []ServiceConfig{
			{Name: "bad-url", LocalPort: 8080, UpstreamURL: "ftp://100.64.0.1:8080"},
		},
	}
	if err := badURL.Validate(); err == nil {
		t.Errorf("Expected error for non-http(s) upstream URL")
	}
}

func TestLoadConfigEnvExpansion(t *testing.T) {
	t.Setenv("TEST_MB_HOST", "my-nas")
	t.Setenv("TEST_MB_PORT", "8096")

	content := `{
		"admin_port": 7350,
		"collision_policy": "auto_rename",
		"services": [
			{
				"name": "jellyfin",
				"local_port": 8096,
				"upstream_url": "http://${TEST_MB_HOST}:${TEST_MB_PORT}",
				"discovery": {
					"server_name": "Jellyfin on ${TEST_MB_HOST}"
				}
			}
		]
	}`

	tmpFile, err := os.CreateTemp("", "meshbeacon-test-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	_ = tmpFile.Close()

	cfg, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.CollisionPolicy != "auto_rename" {
		t.Errorf("Expected collision_policy auto_rename, got: %s", cfg.CollisionPolicy)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(cfg.Services))
	}
	if cfg.Services[0].UpstreamURL != "http://my-nas:8096" {
		t.Errorf("Expected expanded upstream_url http://my-nas:8096, got: %s", cfg.Services[0].UpstreamURL)
	}
	if cfg.Services[0].Discovery.ServerName != "Jellyfin on my-nas" {
		t.Errorf("Expected expanded server_name 'Jellyfin on my-nas', got: %s", cfg.Services[0].Discovery.ServerName)
	}
}

