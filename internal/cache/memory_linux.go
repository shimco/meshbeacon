//go:build linux

package cache

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// detectSystemRAM reads MemTotal from /proc/meminfo on Linux hosts.
func detectSystemRAM() (int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err != nil {
					return 0, err
				}
				return kb * 1024, nil
			}
		}
	}
	return 0, os.ErrNotExist
}
