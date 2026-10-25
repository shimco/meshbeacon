package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shimco/meshbeacon/internal/config"
)

func TestAdminEndpoints(t *testing.T) {
	cfg := &config.Config{
		SchemaVersion: 1,
		AdminPort:     7350,
		AdvertiseAddr: "192.168.88.1",
		Services:      []config.ServiceConfig{},
	}

	srv := NewServer(cfg, "", "v0.8.2", nil, nil, nil)

	// 1. Test Index
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	srv.handleIndex(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /, got %d", rec.Code)
	}

	// 2. Test Healthz
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/healthz", nil)
	srv.handleHealthz(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /healthz, got %d", rec.Code)
	}

	// 3. Test Status in Standby Mode
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/status", nil)
	srv.handleStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /api/status, got %d", rec.Code)
	}

	var status StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("Failed to decode status response: %v", err)
	}
	if status.Mode != "standby" {
		t.Errorf("Expected mode 'standby', got %s", status.Mode)
	}
	if status.Configured {
		t.Errorf("Expected configured=false for empty services")
	}

	// 4. Test Config Save Validation
	badConfig := []byte(`{"services":[{"name":"","local_port":0}]}`)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/config", bytes.NewReader(badConfig))
	srv.handleConfig(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for invalid config, got %d", rec.Code)
	}
}
