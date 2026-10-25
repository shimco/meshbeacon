package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ReleaseAsset describes a GitHub release asset.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// ReleaseInfo encapsulates GitHub release metadata.
type ReleaseInfo struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	PublishedAt time.Time      `json:"published_at"`
	HTMLURL     string         `json:"html_url"`
	Assets      []ReleaseAsset `json:"assets"`
}

// CheckLatestRelease queries GitHub Releases API for the latest version.
func CheckLatestRelease(ctx context.Context, currentVersion string) (*ReleaseInfo, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/shimco/meshbeacon/releases/latest", nil)
	if err != nil {
		return nil, false, fmt.Errorf("could not create release request: %w", err)
	}
	req.Header.Set("User-Agent", "meshbeacon/"+currentVersion)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("failed to query GitHub releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, false, errors.New("GitHub API rate limit reached; check again later")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("GitHub release check returned status %d", resp.StatusCode)
	}

	var rel ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, false, fmt.Errorf("failed to parse release metadata: %w", err)
	}

	isNewer := CompareVersions(rel.TagName, currentVersion) > 0
	return &rel, isNewer, nil
}

// CompareVersions compares two semver strings (e.g. "v0.8.3" vs "v0.8.2").
// Returns 1 if v1 > v2, -1 if v1 < v2, and 0 if equal.
func CompareVersions(v1, v2 string) int {
	clean1 := strings.TrimPrefix(strings.TrimSpace(v1), "v")
	clean2 := strings.TrimPrefix(strings.TrimSpace(v2), "v")

	parts1 := strings.Split(clean1, ".")
	parts2 := strings.Split(clean2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			_, _ = fmt.Sscanf(parts1[i], "%d", &n1)
		}
		if i < len(parts2) {
			_, _ = fmt.Sscanf(parts2[i], "%d", &n2)
		}
		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}
	return 0
}

// IsManagedEnvironment checks if meshbeacon is running inside a container or package-managed host.
func IsManagedEnvironment() (bool, string) {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true, "Docker container (update via docker pull / compose)"
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true, "Podman container (update via podman pull)"
	}
	return false, ""
}

// TargetArchiveName determines the expected asset filename for the host platform.
func TargetArchiveName(tag string) string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map architecture to release conventions
	switch goarch {
	case "amd64":
		goarch = "amd64"
	case "arm64":
		goarch = "arm64"
	case "arm":
		goarch = "armv7"
	case "mipsle":
		goarch = "mipsle"
	case "mips":
		goarch = "mips"
	}

	tagClean := strings.TrimPrefix(tag, "v")
	return fmt.Sprintf("meshbeacon_%s_%s_%s.tar.gz", tagClean, goos, goarch)
}

// DownloadFile streams a URL to a local destination file.
func DownloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "meshbeacon-updater")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d for %s", resp.StatusCode, url)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// VerifySHA256 compares the SHA256 sum of a file with an expected hash string.
func VerifySHA256(filePath, expectedHash string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actualHash := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actualHash, strings.TrimSpace(expectedHash)) {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}
	return nil
}

// ExtractBinaryFromTarGz extracts the 'meshbeacon' executable from a tar.gz archive.
func ExtractBinaryFromTarGz(tarGzPath, destBinaryPath string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to decompress gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag == tar.TypeReg && (header.Name == "meshbeacon" || filepath.Base(header.Name) == "meshbeacon") {
			out, err := os.OpenFile(destBinaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			return out.Close()
		}
	}
	return errors.New("meshbeacon binary not found in archive")
}

// PreflightCheck executes the new binary with -test-config to guarantee it parses config.
func PreflightCheck(newBinaryPath, configPath string) error {
	args := []string{"-test-config"}
	if configPath != "" {
		args = append(args, "-config", configPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, newBinaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("preflight check failed: %v (output: %s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// ExecuteUpdate coordinates safe binary download, hash verification, preflight testing, and atomic replacement.
func ExecuteUpdate(ctx context.Context, currentVersion, configPath string) error {
	isManaged, reason := IsManagedEnvironment()
	if isManaged {
		return fmt.Errorf("cannot self-update: %s", reason)
	}

	rel, isNewer, err := CheckLatestRelease(ctx, currentVersion)
	if err != nil {
		return err
	}
	if !isNewer {
		return fmt.Errorf("already running latest version (%s)", currentVersion)
	}

	targetArchive := TargetArchiveName(rel.TagName)
	var archiveURL, checksumsURL string

	for _, a := range rel.Assets {
		if a.Name == targetArchive {
			archiveURL = a.BrowserDownloadURL
		} else if a.Name == "checksums.txt" {
			checksumsURL = a.BrowserDownloadURL
		}
	}

	if archiveURL == "" {
		return fmt.Errorf("no release asset found for platform %s/%s (%s)", runtime.GOOS, runtime.GOARCH, targetArchive)
	}

	tmpDir, err := os.MkdirTemp("", "meshbeacon-update-*")
	if err != nil {
		return fmt.Errorf("could not create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, targetArchive)
	if err := DownloadFile(ctx, archiveURL, archivePath); err != nil {
		return fmt.Errorf("failed to download release: %w", err)
	}

	// Verify checksum if checksums.txt exists
	if checksumsURL != "" {
		checksumsPath := filepath.Join(tmpDir, "checksums.txt")
		if err := DownloadFile(ctx, checksumsURL, checksumsPath); err == nil {
			chkData, _ := os.ReadFile(checksumsPath)
			lines := strings.Split(string(chkData), "\n")
			for _, line := range lines {
				parts := strings.Fields(line)
				if len(parts) >= 2 && filepath.Base(parts[1]) == targetArchive {
					if err := VerifySHA256(archivePath, parts[0]); err != nil {
						return fmt.Errorf("checksum verification failed: %w", err)
					}
					break
				}
			}
		}
	}

	newBinaryPath := filepath.Join(tmpDir, "meshbeacon.new")
	if err := ExtractBinaryFromTarGz(archivePath, newBinaryPath); err != nil {
		return fmt.Errorf("failed to extract binary: %w", err)
	}

	if err := os.Chmod(newBinaryPath, 0755); err != nil {
		return fmt.Errorf("failed to make binary executable: %w", err)
	}

	// Preflight validation check
	if err := PreflightCheck(newBinaryPath, configPath); err != nil {
		return fmt.Errorf("preflight safety check rejected new version: %w", err)
	}

	// Current binary location
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine current binary path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("could not resolve binary symlink: %w", err)
	}

	// Backup current binary
	bakPath := exePath + ".bak"
	_ = os.Remove(bakPath)
	_ = copyFile(exePath, bakPath)

	// Atomic replacement: rename tmp -> exePath (or copy if rename fails across mounts)
	if err := os.Rename(newBinaryPath, exePath); err != nil {
		if err := copyFile(newBinaryPath, exePath); err != nil {
			return fmt.Errorf("failed to replace binary %s: %w", exePath, err)
		}
	}
	_ = os.Chmod(exePath, 0755)

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
