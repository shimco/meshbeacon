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
		strings.HasSuffix(p, ".mpd") ||
		strings.HasSuffix(p, ".m4b") ||
		strings.HasSuffix(p, ".m4a") ||
		strings.HasSuffix(p, ".mp3") ||
		strings.HasSuffix(p, ".flac") ||
		strings.HasSuffix(p, ".opus") ||
		strings.HasSuffix(p, ".ogg") ||
		strings.HasSuffix(p, ".wav") ||
		strings.HasSuffix(p, ".aac") {
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

	// 4. Audiobookshelf book covers, podcast art & author avatars
	if strings.Contains(p, "/api/items/") && strings.Contains(p, "/cover") ||
		strings.Contains(p, "/api/authors/") && strings.Contains(p, "/image") ||
		strings.Contains(p, "/api/podcasts/") && strings.Contains(p, "/cover") ||
		strings.Contains(p, "/api/books/") && strings.Contains(p, "/cover") {
		return true
	}

	// 5. Kavita & Komga eBook / Manga / Comic covers and thumbnails
	if strings.Contains(p, "/api/image/") ||
		strings.Contains(p, "/api/series/cover/") ||
		strings.Contains(p, "/api/volume/cover/") ||
		strings.Contains(p, "/api/chapter/cover/") ||
		strings.Contains(p, "/thumbnail") && (strings.Contains(p, "/api/v1/series/") || strings.Contains(p, "/api/v1/books/")) {
		return true
	}

	// 6. Mealie recipe photography, food thumbnails & themes
	if strings.Contains(p, "/api/media/recipes/") ||
		strings.Contains(p, "/api/media/themes/") ||
		strings.Contains(p, "/api/media/organizer/") {
		return true
	}

	// 7. Navidrome / Subsonic music album art & artist avatars
	if strings.Contains(p, "/rest/getcoverart") ||
		strings.Contains(p, "/rest/getavatar") ||
		strings.Contains(p, "/api/album/") && strings.Contains(p, "/cover") ||
		strings.Contains(p, "/api/artist/") && strings.Contains(p, "/image") {
		return true
	}

	// 8. RomM retro game box art, cartridge art & screenshots
	if strings.Contains(p, "/api/roms/") && (strings.Contains(p, "/cover") || strings.Contains(p, "/screenshots")) ||
		strings.Contains(p, "/assets/covers/") {
		return true
	}

	// 9. Common static image asset file extensions
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
