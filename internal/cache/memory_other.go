//go:build !linux && !darwin

package cache

import "errors"

// detectSystemRAM fallback for other operating systems.
func detectSystemRAM() (int64, error) {
	return 0, errors.New("system RAM detection not supported on this platform")
}
