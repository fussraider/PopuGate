package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func setupGeoblockTestRouter(t *testing.T) (*gin.Engine, *store.SettingsStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	handler := NewGeoblockHandler(settingsStore, nil)

	r := gin.New()
	r.POST("/geoblock/add", handler.Add)
	r.POST("/geoblock/remove", handler.Remove)
	r.PUT("/geoblock/mode", handler.SetMode)
	r.GET("/geoblock", handler.Get)
	r.POST("/geoblock/clear", handler.Clear)

	return r, settingsStore
}

// --- Get tests ---

func TestGeoblockHandler_Get(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/geoblock", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["mode"] != "blacklist" {
		t.Errorf("expected mode 'blacklist', got %v", resp["mode"])
	}
	if resp["countries"] != "" {
		t.Errorf("expected empty countries by default, got %v", resp["countries"])
	}
}

func TestGeoblockHandler_Get_AfterAdd(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	// Add a country first
	body, _ := json.Marshal(map[string]string{"country": "ru"})
	req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("add: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Now get
	req = httptest.NewRequest(http.MethodGet, "/geoblock", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["countries"] != "ru" {
		t.Errorf("expected countries 'ru', got %v", resp["countries"])
	}
}

// --- Add tests ---

func TestGeoblockAdd_ValidCountry(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	body, _ := json.Marshal(map[string]string{"country": "ru"})
	req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}
	if resp["country"] != "ru" {
		t.Errorf("expected country 'ru', got %v", resp["country"])
	}
}

func TestGeoblockAdd_ValidCountryCodeAlias(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	body, _ := json.Marshal(map[string]string{"country_code": "ru"})
	req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}
	if resp["country"] != "ru" {
		t.Errorf("expected country 'ru', got %v", resp["country"])
	}
}

func TestGeoblockAdd_MultipleCountries(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	// Add first country
	body, _ := json.Marshal(map[string]string{"country": "ru"})
	req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("add ru: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Add second country
	body, _ = json.Marshal(map[string]string{"country": "us"})
	req = httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("add us: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify both are stored
	req = httptest.NewRequest(http.MethodGet, "/geoblock", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	countries := resp["countries"].(string)
	if countries != "ru,us" {
		t.Errorf("expected countries 'ru,us', got %q", countries)
	}
}

func TestGeoblockAdd_InvalidCountry(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	tests := []struct {
		name string
		code string
	}{
		{"invalid xx", "xx"},
		{"path traversal", "../../"},
		{"too short", "a"},
		{"too long", "abc"},
		{"numeric", "12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"country": tt.code})
			req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for %q, got %d: %s", tt.code, w.Code, w.Body.String())
			}
		})
	}
}

func TestGeoblockAdd_ValidationError(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	tests := []struct {
		name string
		body map[string]string
	}{
		{"empty body", map[string]string{}},
		{"numeric country", map[string]string{"country": "12"}},
		{"three letter code", map[string]string{"country": "usa"}},
		{"special chars", map[string]string{"country": "r!"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// --- Remove tests ---

func TestGeoblockRemove_ValidCountry(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	// Add two countries first
	for _, code := range []string{"ru", "us"} {
		body, _ := json.Marshal(map[string]string{"country": code})
		req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("add %s: expected 200, got %d", code, w.Code)
		}
	}

	// Remove one
	body, _ := json.Marshal(map[string]string{"country": "ru"})
	req := httptest.NewRequest(http.MethodPost, "/geoblock/remove", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}
	if resp["country"] != "ru" {
		t.Errorf("expected country 'ru', got %v", resp["country"])
	}

	// Verify only "us" remains
	req = httptest.NewRequest(http.MethodGet, "/geoblock", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var getResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &getResp)
	if getResp["countries"] != "us" {
		t.Errorf("expected countries 'us', got %v", getResp["countries"])
	}
}

func TestGeoblockRemove_ValidationError(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	// Missing country field
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/geoblock/remove", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGeoblockRemove_NonexistentCountry(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	// Add one country
	body, _ := json.Marshal(map[string]string{"country": "ru"})
	req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Remove a different country (not in list) — should still succeed
	body, _ = json.Marshal(map[string]string{"country": "de"})
	req = httptest.NewRequest(http.MethodPost, "/geoblock/remove", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// "ru" should still be there
	req = httptest.NewRequest(http.MethodGet, "/geoblock", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["countries"] != "ru" {
		t.Errorf("expected 'ru' to remain, got %v", resp["countries"])
	}
}

// --- Clear tests ---

func TestGeoblockHandler_Clear(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	// Add some countries first
	for _, code := range []string{"ru", "us", "de"} {
		body, _ := json.Marshal(map[string]string{"country": code})
		req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	// Clear
	req := httptest.NewRequest(http.MethodPost, "/geoblock/clear", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}

	// Verify countries are cleared
	req = httptest.NewRequest(http.MethodGet, "/geoblock", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var getResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &getResp)
	if getResp["countries"] != "" {
		t.Errorf("expected empty countries after clear, got %v", getResp["countries"])
	}
}

// --- SetMode tests ---

func TestGeoblockHandler_SetMode_ValidBlacklist(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	body, _ := json.Marshal(map[string]string{"mode": "blacklist"})
	req := httptest.NewRequest(http.MethodPut, "/geoblock/mode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}
	if resp["mode"] != "blacklist" {
		t.Errorf("expected mode 'blacklist', got %v", resp["mode"])
	}
}

func TestGeoblockHandler_SetMode_ValidWhitelist(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	body, _ := json.Marshal(map[string]string{"mode": "whitelist"})
	req := httptest.NewRequest(http.MethodPut, "/geoblock/mode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["mode"] != "whitelist" {
		t.Errorf("expected mode 'whitelist', got %v", resp["mode"])
	}

	// Verify persisted via Get
	req = httptest.NewRequest(http.MethodGet, "/geoblock", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var getResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &getResp)
	if getResp["mode"] != "whitelist" {
		t.Errorf("expected mode 'whitelist' persisted, got %v", getResp["mode"])
	}
}

func TestGeoblockHandler_SetMode_InvalidMode(t *testing.T) {
	r, _ := setupGeoblockTestRouter(t)

	tests := []string{"invalid", "allowlist", "blocklist", "BLACKLIST", "WHITELIST", ""}
	for _, mode := range tests {
		t.Run(mode, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"mode": mode})
			req := httptest.NewRequest(http.MethodPut, "/geoblock/mode", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for mode %q, got %d: %s", mode, w.Code, w.Body.String())
			}
		})
	}
}

// --- splitCountries helper tests ---

func TestSplitCountries(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"ru", []string{"ru"}},
		{"ru,us", []string{"ru", "us"}},
		{"ru, us, de", []string{"ru", "us", "de"}},
		{",ru,,us,", []string{"ru", "us"}},
		{"ru,", []string{"ru"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitCountries(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitCountries(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitCountries(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// --- stringsJoinComma helper tests ---

func TestStringsJoinComma(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{"nil slice", nil, ""},
		{"empty slice", []string{}, ""},
		{"single element", []string{"ru"}, "ru"},
		{"multiple elements", []string{"ru", "us", "de"}, "ru,us,de"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringsJoinComma(tt.parts)
			if got != tt.want {
				t.Errorf("stringsJoinComma(%v) = %q, want %q", tt.parts, got, tt.want)
			}
		})
	}
}

func TestGeoblockAdd_SettingsLoadFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	// Close DB to cause settings.Load to fail
	_ = db.Close()

	handler := NewGeoblockHandler(settingsStore, nil)
	r := gin.New()
	r.POST("/geoblock/add", handler.Add)

	body, _ := json.Marshal(map[string]string{"country": "ru"})
	req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when settings.Load fails, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGeoblockRemove_SettingsLoadFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	_ = db.Close()

	handler := NewGeoblockHandler(settingsStore, nil)
	r := gin.New()
	r.POST("/geoblock/remove", handler.Remove)

	body, _ := json.Marshal(map[string]string{"country": "ru"})
	req := httptest.NewRequest(http.MethodPost, "/geoblock/remove", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when settings.Load fails, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGeoblockHandler_Unavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	
	// Create a real GeoblockService. On macOS, this will be unavailable because there is no iptables/ipset.
	instancesStore := store.NewInstanceStore(db)
	cacheStore := store.NewGeoblockCacheStore(db)
	geoSvc := service.NewGeoblockService(settingsStore, instancesStore, cacheStore)
	
	// If by any chance they are installed (e.g. running on Linux CI with iptables), we skip or check the status.
	available, _ := geoSvc.IsAvailable()
	if available {
		t.Skip("iptables/ipset are available, skipping unavailable test")
	}

	handler := NewGeoblockHandler(settingsStore, geoSvc)

	r := gin.New()
	r.POST("/geoblock/add", handler.Add)
	r.GET("/geoblock", handler.Get)

	// Test Get shows available = false
	req := httptest.NewRequest(http.MethodGet, "/geoblock", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["available"] != false {
		t.Errorf("expected available to be false, got %v", resp["available"])
	}

	// Test Add returns 400 Bad Request
	body, _ := json.Marshal(map[string]string{"country": "ru"})
	req = httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d: %s", w.Code, w.Body.String())
	}
	
	var errResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &errResp)
	if !strings.Contains(errResp["error"].(string), "geoblocking is not available") {
		t.Errorf("expected error message about availability, got: %v", errResp["error"])
	}
}
