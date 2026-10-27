package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetectJellyfinInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/System/Info/Public" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"Id":"jellyfin-uuid-1234","ServerName":"My Jellyfin"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	id, name, err := DetectJellyfinInfo(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("DetectJellyfinInfo failed: %v", err)
	}
	if id != "jellyfin-uuid-1234" {
		t.Errorf("Expected id jellyfin-uuid-1234, got %q", id)
	}
	if name != "My Jellyfin" {
		t.Errorf("Expected name 'My Jellyfin', got %q", name)
	}
}

func TestDetectPlexInfoJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"MediaContainer":{"machineIdentifier":"plex-machine-id-abcd"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	id, err := DetectPlexInfo(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("DetectPlexInfo JSON failed: %v", err)
	}
	if id != "plex-machine-id-abcd" {
		t.Errorf("Expected id plex-machine-id-abcd, got %q", id)
	}
}

func TestDetectPlexInfoXMLFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><MediaContainer size="0" machineIdentifier="plex-machine-id-xml-5678" version="1.40"></MediaContainer>`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	id, err := DetectPlexInfo(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("DetectPlexInfo XML fallback failed: %v", err)
	}
	if id != "plex-machine-id-xml-5678" {
		t.Errorf("Expected id plex-machine-id-xml-5678, got %q", id)
	}
}

func TestFallbackServerID(t *testing.T) {
	id1 := FallbackServerID("http://192.168.1.100:8096")
	id2 := FallbackServerID("http://192.168.1.100:8096")
	id3 := FallbackServerID("http://192.168.1.101:8096")

	if id1 == "" {
		t.Fatalf("FallbackServerID returned empty string")
	}
	if id1 != id2 {
		t.Errorf("Expected deterministic ID, got %q vs %q", id1, id2)
	}
	if id1 == id3 {
		t.Errorf("Expected different IDs for different upstream URLs")
	}
}
