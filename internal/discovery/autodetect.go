package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type jellyfinPublicInfo struct {
	Id         string `json:"Id"`
	ServerName string `json:"ServerName"`
}

type plexIdentityJSON struct {
	MediaContainer struct {
		MachineIdentifier string `json:"machineIdentifier"`
	} `json:"MediaContainer"`
}

// DetectJellyfinInfo queries the unauthenticated /System/Info/Public endpoint of an upstream
// Jellyfin server to extract its real Server ID and Server Name.
func DetectJellyfinInfo(ctx context.Context, upstreamURL string) (string, string, error) {
	baseURL := strings.TrimRight(upstreamURL, "/")
	reqURL := baseURL + "/System/Info/Public"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", "", err
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to query Jellyfin info at %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("jellyfin returned unexpected HTTP %d from %s", resp.StatusCode, reqURL)
	}

	var info jellyfinPublicInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", "", fmt.Errorf("failed to decode Jellyfin info JSON: %w", err)
	}

	return info.Id, info.ServerName, nil
}

// DetectPlexInfo queries the unauthenticated /identity endpoint of an upstream
// Plex Media Server to extract its unique machineIdentifier.
func DetectPlexInfo(ctx context.Context, upstreamURL string) (string, error) {
	baseURL := strings.TrimRight(upstreamURL, "/")
	reqURL := baseURL + "/identity"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to query Plex identity at %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("plex returned unexpected HTTP %d from %s", resp.StatusCode, reqURL)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("failed to read Plex identity response: %w", err)
	}

	// 1. Fast JSON decode (Plex supports JSON when Accept: application/json is provided)
	var jsonResp plexIdentityJSON
	if err := json.Unmarshal(bodyBytes, &jsonResp); err == nil && jsonResp.MediaContainer.MachineIdentifier != "" {
		return jsonResp.MediaContainer.MachineIdentifier, nil
	}

	// 2. Fallback: Parse XML string without encoding/xml to keep binary footprint minimal
	if id := extractMachineIDFromXML(string(bodyBytes)); id != "" {
		return id, nil
	}

	return "", fmt.Errorf("no machineIdentifier found in Plex /identity response")
}

func extractMachineIDFromXML(body string) string {
	idx := strings.Index(body, `machineIdentifier="`)
	if idx == -1 {
		return ""
	}
	sub := body[idx+len(`machineIdentifier="`):]
	end := strings.IndexByte(sub, '"')
	if end == -1 {
		return ""
	}
	return sub[:end]
}

// FallbackServerID generates a stable, deterministic UUID-like hex ID based on the upstream URL
// if the upstream server is temporarily unreachable on cold boot.
func FallbackServerID(upstreamURL string) string {
	h := sha256.Sum256([]byte(upstreamURL))
	return hex.EncodeToString(h[:16])
}
