package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// DiscoveryConfig defines service advertisement and autodiscovery parameters.
type DiscoveryConfig struct {
	Type       string            `json:"type"`        // "jellyfin", "plex", "immich", "mdns", "none"
	Port       int               `json:"port"`        // Discovery port (e.g. 7359 for Jellyfin, 32414 for Plex)
	ServerName string            `json:"server_name"` // Broadcast display name / mDNS instance name
	ServerID   string            `json:"server_id"`   // Unique server identifier (auto-detected for Jellyfin/Plex)
	Service    string            `json:"service"`     // mDNS service type, e.g. "_immich._tcp", "_ipp._tcp", "_hap._tcp"
	TXT        map[string]string `json:"txt"`         // Custom mDNS TXT record key-value pairs
}

// NormalizeServiceType formats the service identifier into RFC 6763 DNS-SD format (e.g. "_immich._tcp").
func (d *DiscoveryConfig) NormalizeServiceType(fallbackName string) string {
	svc := strings.TrimSpace(d.Service)
	if svc == "" {
		svc = strings.TrimSpace(fallbackName)
	}
	if !strings.HasPrefix(svc, "_") {
		svc = "_" + svc
	}
	if !strings.HasSuffix(svc, "._tcp") && !strings.HasSuffix(svc, "._udp") {
		svc = svc + "._tcp"
	}
	return svc
}

// FormattedTXT returns the configured TXT key-value pairs as sorted strings.
func (d *DiscoveryConfig) FormattedTXT() []string {
	if len(d.TXT) == 0 {
		return []string{"path=/"}
	}
	records := make([]string, 0, len(d.TXT))
	for k, v := range d.TXT {
		if v != "" {
			records = append(records, fmt.Sprintf("%s=%s", k, v))
		} else {
			records = append(records, k)
		}
	}
	sort.Strings(records)
	return records
}

// ServiceConfig defines a single media service to bridge into the local LAN.
type ServiceConfig struct {
	Name        string          `json:"name"`         // e.g. "jellyfin", "plex", "immich", "cups"
	LocalPort   int             `json:"local_port"`   // e.g. 8096, 32400, 2283, 631
	UpstreamURL string          `json:"upstream_url"` // e.g. "http://mediaserver.your-tailnet.ts.net:8096"
	Discovery   DiscoveryConfig `json:"discovery"`
}

// CacheConfig defines the dual-tier asset caching settings.
type CacheConfig struct {
	Enabled  bool   `json:"enabled"`
	RAMSize  string `json:"ram_size"`  // "auto", "64MB", "off"
	DiskDir  string `json:"disk_dir"`  // "" (disabled), or path to directory
	DiskSize string `json:"disk_size"` // "auto", "5GB"
	TTL      string `json:"ttl"`       // "auto" (30d RAM, 90d disk), "30d", "90d", "180d", "1y"
}

// Config represents the complete Meshbeacon runtime configuration.
type Config struct {
	AdvertiseAddr string          `json:"advertise_addr"` // e.g. "http://192.168.1.100" or "auto"
	Cache         CacheConfig     `json:"cache"`
	Services      []ServiceConfig `json:"services"`
}

// LoadConfig reads configuration from a JSON file path.
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read config file: %w", err)
	}

	var cfg Config
	// Defaults
	cfg.Cache.Enabled = true
	cfg.Cache.RAMSize = "auto"
	cfg.Cache.TTL = "auto"

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid json in config file: %w", err)
	}

	if cfg.Cache.RAMSize == "" {
		cfg.Cache.RAMSize = "auto"
	}
	if cfg.Cache.TTL == "" {
		cfg.Cache.TTL = "auto"
	}

	return &cfg, nil
}

// DefaultConfig generates a sane multi-service configuration if no file is passed.
func DefaultConfig(advertiseHost, upstreamJellyfin, upstreamPlex, upstreamImmich string) *Config {
	cfg := &Config{
		AdvertiseAddr: advertiseHost,
		Cache: CacheConfig{
			Enabled:  true,
			RAMSize:  "auto",
			DiskDir:  "",
			DiskSize: "auto",
			TTL:      "auto",
		},
		Services: make([]ServiceConfig, 0),
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

	if upstreamImmich != "" {
		sc := ServiceConfig{
			Name:        "immich",
			LocalPort:   2283,
			UpstreamURL: upstreamImmich,
		}
		sc.Discovery.Type = "immich"
		sc.Discovery.Service = "_immich._tcp"
		sc.Discovery.ServerName = "Immich Photos"
		sc.Discovery.TXT = map[string]string{"path": "/api"}
		cfg.Services = append(cfg.Services, sc)
	}

	return cfg
}
