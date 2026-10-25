package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	UpstreamURL string          `json:"upstream_url"` // e.g. "http://media-server.your-tailnet.ts.net:8096"
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
	SchemaVersion int             `json:"schema_version,omitempty"` // Configuration schema version (e.g. 1)
	AdminPort     int             `json:"admin_port,omitempty"`     // Web dashboard & setup port (default: 7350)
	AutoUpdate    bool            `json:"auto_update,omitempty"`    // Enable daily automatic updates on standalone installs
	AdvertiseAddr string          `json:"advertise_addr"`           // e.g. "http://192.168.1.100" or "auto"
	Cache         CacheConfig     `json:"cache"`
	Services      []ServiceConfig `json:"services"`
}

// IsPlaceholderURL checks if a target upstream URL is unconfigured template filler.
func IsPlaceholderURL(rawURL string) bool {
	u := strings.TrimSpace(strings.ToLower(rawURL))
	if u == "" {
		return true
	}
	placeholders := []string{
		"media-server:8096",
		"media-server:32400",
		"media-server.your-tailnet.ts.net",
		"photos.your-tailnet.ts.net",
		"audiobooks.your-tailnet.ts.net",
		"books.your-tailnet.ts.net",
		"recipes.your-tailnet.ts.net",
		"cups.your-tailnet.ts.net",
		"example.com",
		"your-tailnet",
		"http://media-server",
		"https://media-server",
	}
	for _, p := range placeholders {
		if strings.Contains(u, p) {
			return true
		}
	}
	return false
}

// IsConfigured returns true if at least one service has a genuine, non-placeholder upstream target.
func (c *Config) IsConfigured() bool {
	if len(c.Services) == 0 {
		return false
	}
	for _, s := range c.Services {
		if !IsPlaceholderURL(s.UpstreamURL) {
			return true
		}
	}
	return false
}

// Validate checks for structural, port, and schema errors in the configuration.
func (c *Config) Validate() error {
	if c.AdminPort == 0 {
		c.AdminPort = 7350
	} else if c.AdminPort < 1 || c.AdminPort > 65535 {
		return fmt.Errorf("invalid admin_port: %d (must be between 1 and 65535)", c.AdminPort)
	}

	usedPorts := make(map[int]string)
	usedPorts[c.AdminPort] = "admin/dashboard"

	for i, s := range c.Services {
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("services[%d]: service name cannot be empty", i)
		}
		if s.LocalPort < 1 || s.LocalPort > 65535 {
			return fmt.Errorf("services[%d] (%s): invalid local_port %d", i, s.Name, s.LocalPort)
		}
		if owner, exists := usedPorts[s.LocalPort]; exists {
			return fmt.Errorf("services[%d] (%s): local_port %d conflicts with %s", i, s.Name, s.LocalPort, owner)
		}
		usedPorts[s.LocalPort] = s.Name

		if strings.TrimSpace(s.UpstreamURL) != "" {
			trimmed := strings.TrimSpace(s.UpstreamURL)
			if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") &&
				!strings.HasPrefix(trimmed, "tcp://") && !strings.HasPrefix(trimmed, "smb://") {
				return fmt.Errorf("services[%d] (%s): upstream_url %q must begin with http://, https://, tcp://, or smb://", i, s.Name, s.UpstreamURL)
			}
		}
	}

	if c.Cache.DiskDir != "" {
		clean := filepath.Clean(strings.TrimSpace(c.Cache.DiskDir))
		if !filepath.IsAbs(clean) {
			return fmt.Errorf("cache.disk_dir %q must be an absolute path", c.Cache.DiskDir)
		}
		forbidden := []string{
			"/", "/bin", "/sbin", "/usr", "/usr/bin", "/usr/sbin", "/etc", "/root",
			"/boot", "/sys", "/proc", "/dev", "/lib", "/lib64", "/var", "/run",
		}
		for _, f := range forbidden {
			if clean == f {
				return fmt.Errorf("cache.disk_dir cannot be set to critical system directory %q", clean)
			}
		}
	}

	return nil
}

// SaveConfig atomically writes the configuration to disk.
func SaveConfig(filePath string, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("cannot save invalid config: %w", err)
	}

	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = 1
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal config to json: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("could not create config directory %s: %w", dir, err)
	}

	tmpFile, err := os.CreateTemp(dir, "meshbeacon-*.json.tmp")
	if err != nil {
		return fmt.Errorf("could not create temporary config file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("could not write temporary config: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("could not sync temporary config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("could not close temporary config: %w", err)
	}

	if err := os.Rename(tmpName, filePath); err != nil {
		return fmt.Errorf("could not atomically replace %s: %w", filePath, err)
	}

	return nil
}

// LoadConfig reads configuration from a JSON file path.
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read config file: %w", err)
	}

	var cfg Config
	// Defaults
	cfg.SchemaVersion = 1
	cfg.AdminPort = 7350
	cfg.Cache.Enabled = true
	cfg.Cache.RAMSize = "auto"
	cfg.Cache.TTL = "auto"

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid json in config file: %w", err)
	}

	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = 1
	}
	if cfg.AdminPort == 0 {
		cfg.AdminPort = 7350
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
