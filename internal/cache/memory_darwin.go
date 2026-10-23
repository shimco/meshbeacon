//go:build darwin

package cache

import (
	"encoding/binary"
	"fmt"
	"syscall"
)

// detectSystemRAM reads hw.memsize via sysctl on macOS/Darwin.
func detectSystemRAM() (int64, error) {
	s, err := syscall.Sysctl("hw.memsize")
	if err != nil {
		return 0, err
	}
	if len(s) == 8 {
		mem := binary.LittleEndian.Uint64([]byte(s))
		return int64(mem), nil
	}
	return 0, fmt.Errorf("unexpected sysctl format for hw.memsize")
}
