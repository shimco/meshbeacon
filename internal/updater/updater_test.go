package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"v0.8.3", "v0.8.2", 1},
		{"0.8.3", "0.8.2", 1},
		{"v0.8.2", "v0.8.3", -1},
		{"v0.8.2", "v0.8.2", 0},
		{"v1.0.0", "v0.9.9", 1},
		{"v0.9.9", "v1.0.0", -1},
		{"v0.8.10", "v0.8.9", 1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			actual := CompareVersions(tt.v1, tt.v2)
			if actual != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d, expected %d", tt.v1, tt.v2, actual, tt.expected)
			}
		})
	}
}

func TestVerifySHA256(t *testing.T) {
	tmpDir := t.TempDir()
	sampleFile := filepath.Join(tmpDir, "sample.txt")
	content := []byte("meshbeacon rocks")

	if err := os.WriteFile(sampleFile, content, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	h := sha256.Sum256(content)
	expectedHash := hex.EncodeToString(h[:])

	if err := VerifySHA256(sampleFile, expectedHash); err != nil {
		t.Errorf("VerifySHA256 failed on matching hash: %v", err)
	}

	if err := VerifySHA256(sampleFile, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Errorf("VerifySHA256 should have failed on mismatching hash")
	}
}
