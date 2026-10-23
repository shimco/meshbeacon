package cache

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const (
	KB = int64(1024)
	MB = int64(1024 * KB)
	GB = int64(1024 * MB)
)

// DetectTotalRAM returns total physical RAM in bytes, or a sensible estimate if undetected.
func DetectTotalRAM() int64 {
	ram, err := detectSystemRAM()
	if err == nil && ram > 0 {
		return ram
	}
	// Fallback estimate: 1GB
	return 1 * GB
}

// CalculateAutoRAMCacheSize computes a tiered RAM cache budget based on hardware capacity.
// Scaled aggressively on hosts with ample headroom, while conservative on micro-routers.
func CalculateAutoRAMCacheSize(totalRAM int64) int64 {
	switch {
	case totalRAM <= 128*MB:
		return 8 * MB // GL-MT300N Mango (128MB RAM) -> 8MB (~200 posters)
	case totalRAM <= 512*MB:
		return 32 * MB // Pi Zero 2W / GL-MT3000 (512MB RAM) -> 32MB (~800 posters)
	case totalRAM <= 1*GB:
		return 96 * MB // Raspberry Pi 3 (1GB RAM) -> 96MB (~2,500 posters)
	case totalRAM <= 2*GB:
		return 192 * MB // Raspberry Pi 4 (2GB RAM) -> 192MB (~4,800 posters)
	case totalRAM <= 4*GB:
		return 256 * MB // Raspberry Pi 4/5 (4GB RAM) -> 256MB (~6,500 posters)
	default:
		return 512 * MB // Homelabs, Mini-PCs, Workstations (>4GB RAM) -> 512MB (~13,000 posters)
	}
}

// CalculateAutoRAMCacheSizeWithDisk returns a tiered hot-buffer budget when
// secondary persistent disk caching is enabled:
// - Micro-routers (<=128MB): 8MB
// - Mini SBCs (<=512MB): 32MB
// - Moderate SBCs (1GB-2GB): 64MB (~1,600 posters)
// - Capable SBCs & Servers (>=4GB): 128MB (~3,200 posters)
func CalculateAutoRAMCacheSizeWithDisk(totalRAM int64) int64 {
	switch {
	case totalRAM <= 128*MB:
		return 8 * MB
	case totalRAM <= 512*MB:
		return 32 * MB
	case totalRAM < 4*GB:
		return 64 * MB
	default:
		return 128 * MB
	}
}

// ParseSize converts a user size string (e.g. "auto", "64MB", "2GB", "off") to bytes.
func ParseSize(str string, autoSize int64) (int64, error) {
	s := strings.TrimSpace(strings.ToLower(str))
	if s == "" || s == "auto" {
		return autoSize, nil
	}
	if s == "off" || s == "0" || s == "disabled" || s == "none" || s == "false" {
		return 0, nil
	}

	var numStr strings.Builder
	var unitStr strings.Builder

	for _, r := range s {
		if unicode.IsDigit(r) || r == '.' {
			numStr.WriteRune(r)
		} else if !unicode.IsSpace(r) {
			unitStr.WriteRune(r)
		}
	}

	val, err := strconv.ParseFloat(numStr.String(), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size number: %q", str)
	}

	multiplier := int64(1)
	switch unitStr.String() {
	case "", "b", "bytes":
		multiplier = 1
	case "k", "kb", "kib":
		multiplier = KB
	case "m", "mb", "mib":
		multiplier = MB
	case "g", "gb", "gib":
		multiplier = GB
	case "t", "tb", "tib":
		multiplier = 1024 * GB
	default:
		return 0, fmt.Errorf("unknown size unit in %q", str)
	}

	return int64(val * float64(multiplier)), nil
}

// FormatSize formats a byte count into a human-readable string (e.g. "96 MB").
func FormatSize(bytes int64) string {
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.0f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.0f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
