package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// ServiceConfig defines a single media service to bridge into the local LAN.
type ServiceConfig struct {
	Name        string `json:"name"`         // e.g. "jellyfin", "plex", "navidrome"
	LocalPort   int    `json:"local_port"`   // e.g. 8096, 32400, 4533
	UpstreamURL string `json:"upstream_url"` // e.g. "http://mediaserver.your-tailnet.ts.net:8096"
	Discovery   struct {
		Type       string `json:"type"`        // "jellyfin", "plex", "none"
		Port       int    `json:"port"`        // Discovery port, e.g. 7359 or 32414
		ServerName string `json:"server_name"` // Broadcast display name (auto-detected if blank)
		ServerID   string `json:"server_id"`   // Unique server identifier (auto-detected if blank)
	} `json:"discovery"`
}

// Config represents the complete Tailbeacon runtime configuration.
type Config struct {
	AdvertiseAddr string          `json:"advertise_addr"` // e.g. "http://192.168.1.100" or "auto"
	Services      []ServiceConfig `json:"services"`
}

// LoadConfig reads configuration from a JSON file path.
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid json in config file: %w", err)
	}

	return &cfg, nil
}

// DefaultConfig generates a sane multi-service configuration if no file is passed.
func DefaultConfig(advertiseHost string, upstreamJellyfin string, upstreamPlex string) *Config {
	cfg := &Config{
		AdvertiseAddr: advertiseHost,
		Services:      make([]ServiceConfig, 0),
	}

	if upstreamJellyfin != "" {
		sc := ServiceConfig{
			Name:        "jellyfin",
			LocalPort:   8096,
			UpstreamURL: upstreamJellyfin,
		}
		sc.Discovery.Type = "jellyfin"
		sc.Discovery.Port = 7359
		sc.Discovery.ServerName = "" // Auto-detected from upstream
		sc.Discovery.ServerID = ""   // Auto-detected from upstream
		cfg.Services = append(cfg.Services, sc)
	}

	if upstreamPlex != "" {
		sc := ServiceConfig{
			Name:        "plex",
			LocalPort:   32400,
			UpstreamURL: upstreamPlex,
		}
		sc.Discovery.Type = "plex"
		sc.Discovery.Port = 32414
		sc.Discovery.ServerName = "" // Auto-detected from upstream
		sc.Discovery.ServerID = ""   // Auto-detected from upstream
		cfg.Services = append(cfg.Services, sc)
	}

	return cfg
}
