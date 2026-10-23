package cache

import (
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// MinDiskFreeSpaceFloor is the minimum filesystem free space reserve (50MB).
	// If underlying drive free space drops below this, disk caching pauses to prevent host failure.
	MinDiskFreeSpaceFloor = 50 * MB

	// DefaultMaxAutoDisk is the maximum cache allocation when disk_size is "auto" on large drives (10GB).
	DefaultMaxAutoDisk = 10 * GB

	// DefaultMinAutoDisk is the minimum auto cache allocation for small/embedded drives (250MB).
	DefaultMinAutoDisk = 250 * MB
)

// AvailableDiskSpace queries the underlying filesystem for free bytes available to unprivileged processes.
func AvailableDiskSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

// CalculateAutoDiskCacheSize computes a safe disk cache quota based on detected free drive space:
// - Clamps to 50% of available free space to ensure host breathing room
// - Caps at 10 GB on large NVMe drives (storing ~20,000+ photo previews or 100,000+ posters)
// - Minimum of 250 MB (or available free space minus 50MB reserve on tight thumb drives)
func CalculateAutoDiskCacheSize(freeBytes int64) int64 {
	if freeBytes <= 0 {
		return DefaultMinAutoDisk
	}

	halfFree := freeBytes / 2
	switch {
	case halfFree < DefaultMinAutoDisk:
		if freeBytes > MinDiskFreeSpaceFloor {
			return freeBytes - MinDiskFreeSpaceFloor
		}
		return DefaultMinAutoDisk
	case halfFree > DefaultMaxAutoDisk:
		return DefaultMaxAutoDisk
	default:
		return halfFree
	}
}

// ParseTTL parses human-readable duration strings like "30d", "90d", "180d", "1y", "720h".
func ParseTTL(str string, defaultDuration time.Duration) time.Duration {
	s := strings.TrimSpace(strings.ToLower(str))
	if s == "" {
		return defaultDuration
	}

	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		if days, err := strconv.Atoi(daysStr); err == nil && days > 0 {
			return time.Duration(days) * 24 * time.Hour
		}
	} else if strings.HasSuffix(s, "w") {
		weeksStr := strings.TrimSuffix(s, "w")
		if weeks, err := strconv.Atoi(weeksStr); err == nil && weeks > 0 {
			return time.Duration(weeks) * 7 * 24 * time.Hour
		}
	} else if strings.HasSuffix(s, "y") {
		yearsStr := strings.TrimSuffix(s, "y")
		if years, err := strconv.Atoi(yearsStr); err == nil && years > 0 {
			return time.Duration(years) * 365 * 24 * time.Hour
		}
	}

	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}

	return defaultDuration
}
