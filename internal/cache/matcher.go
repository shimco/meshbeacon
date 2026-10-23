package cache

import (
	"net/http"
	"strings"
)

// IsCacheableAsset checks if an incoming HTTP request targets a static media asset (posters, thumbnails, fanart).
func IsCacheableAsset(serviceName, method, path string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}

	p := strings.ToLower(path)

	// Exclude video/audio streaming streams immediately
	if strings.Contains(p, "/videos/") ||
		strings.Contains(p, "/audio/") ||
		strings.Contains(p, "/stream") ||
		strings.Contains(p, "/subtitles/") ||
		strings.HasSuffix(p, ".m3u8") ||
		strings.HasSuffix(p, ".ts") ||
		strings.HasSuffix(p, ".mp4") ||
		strings.HasSuffix(p, ".mkv") ||
		strings.HasSuffix(p, ".mpd") {
		return false
	}

	// 1. Jellyfin / Emby artwork paths
	if strings.Contains(p, "/images/") ||
		strings.Contains(p, "/userimages/") ||
		strings.Contains(p, "/branding/css") ||
		strings.Contains(p, "/branding/configurationpage") {
		return true
	}

	// 2. Plex artwork and transcode thumbnails
	if strings.HasPrefix(p, "/photo/:/transcode") ||
		strings.Contains(p, "/thumb/") ||
		strings.HasSuffix(p, "/thumb") ||
		strings.HasSuffix(p, "/art") ||
		strings.Contains(p, "/art/") ||
		strings.Contains(p, "/media/providers/") {
		return true
	}

	// 3. Immich thumbnails & previews
	if strings.Contains(p, "/api/assets/") && strings.Contains(p, "/thumbnail") ||
		strings.Contains(p, "/api/asset/thumbnail/") {
		return true
	}

	// 4. Common static image asset file extensions
	if strings.HasSuffix(p, ".jpg") ||
		strings.HasSuffix(p, ".jpeg") ||
		strings.HasSuffix(p, ".png") ||
		strings.HasSuffix(p, ".webp") ||
		strings.HasSuffix(p, ".svg") ||
		strings.HasSuffix(p, ".ico") {
		return true
	}

	return false
}
