package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mcp_server_scraper_googlemaps/internal/dataset"
	"mcp_server_scraper_googlemaps/internal/models"
)

func TestHTTPInitialize(t *testing.T) {
	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	req := newAuthorizedRequest(t, http.MethodPost, "/mcp", body)
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", resp.Result)
	}
	if result["protocolVersion"] != latestProtocolVersion {
		t.Fatalf("protocolVersion = %v, want %s", result["protocolVersion"], latestProtocolVersion)
	}
}

func TestHTTPNotificationReturnsAccepted(t *testing.T) {
	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`)
	req := newAuthorizedRequest(t, http.MethodPost, "/mcp", body)
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
}

func TestHTTPResponseOnlyReturnsAccepted(t *testing.T) {
	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":10,"result":{}}`)
	req := newAuthorizedRequest(t, http.MethodPost, "/mcp", body)
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
}

func TestHTTPToolsList(t *testing.T) {
	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("scrape_google_maps")) {
		t.Fatalf("response does not list scrape_google_maps: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("update_place_actions")) {
		t.Fatalf("response does not list dataset action tools: %s", rec.Body.String())
	}
}

func TestHTTPToolsListDoesNotRequireBearerToken(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "")
	t.Setenv("MCP_BEARER_TOKEN", "")

	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("list_dataset_places")) {
		t.Fatalf("response does not list dataset tools: %s", rec.Body.String())
	}
}

func TestHTTPToolsListAllowsChatGPTOrigin(t *testing.T) {
	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Origin", "https://chatgpt.com")
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("scrape_google_maps")) {
		t.Fatalf("response does not list scrape_google_maps: %s", rec.Body.String())
	}
}

func TestHTTPGetReturnsMethodNotAllowed(t *testing.T) {
	server := New(nil, nil, nil, nil)
	req := newAuthorizedRequest(t, http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHTTPToolCallBearerTokenRequired(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "")
	t.Setenv("MCP_BEARER_TOKEN", "secret-token")
	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"extract_contacts_from_html","arguments":{"html":"<html></html>"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("WWW-Authenticate header is empty")
	}
}

func TestHTTPBearerTokenInvalid(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "")
	t.Setenv("MCP_BEARER_TOKEN", "secret-token")
	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"extract_contacts_from_html","arguments":{"html":"<html></html>"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHTTPBearerTokenValid(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "")
	t.Setenv("MCP_BEARER_TOKEN", "secret-token")
	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"extract_contacts_from_html","arguments":{"html":"<html>Contact contact@example.com</html>"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHTTPToolCallAPIKeyHeaderValid(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "")
	t.Setenv("MCP_BEARER_TOKEN", "secret-token")
	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"extract_contacts_from_html","arguments":{"html":"<html>Contact contact@example.com</html>"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("X-API-Key", "secret-token")
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHTTPBearerTokenFailsClosedWhenEnvEmpty(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "")
	t.Setenv("MCP_BEARER_TOKEN", "")

	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"extract_contacts_from_html","arguments":{"html":"<html></html>"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("server authentication is not configured")) {
		t.Fatalf("response does not describe missing server auth: %s", rec.Body.String())
	}
}

func TestHTTPToolCallWithNilScraperReturnsToolError(t *testing.T) {
	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"scrape_google_maps","arguments":{"searchQueries":["pizza"],"maxPlacesPerQuery":1}}}`)
	req := newAuthorizedRequest(t, http.MethodPost, "/mcp", body)
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("scraper is not configured")) {
		t.Fatalf("response does not contain nil scraper error: %s", rec.Body.String())
	}
}

func TestHTTPDatasetActionTools(t *testing.T) {
	store := dataset.New(t.TempDir(), nil)
	ctx := t.Context()
	place := models.PlaceData{
		Query:         "pizzarias em Curitiba",
		Name:          "Pizza Central",
		Address:       stringPtrForMCPTest("Rua A, 123"),
		GoogleMapsURL: "https://www.google.com/maps/place/pizza-central",
		Emails:        []string{},
		Phones:        []string{},
		SocialLinks:   models.EmptySocialLinks(),
	}
	if err := store.SaveExtraction(ctx, models.Input{SearchQueries: []string{"pizzarias em Curitiba"}}, []models.PlaceData{place}); err != nil {
		t.Fatalf("SaveExtraction() error = %v", err)
	}
	list, err := store.ListPlaces(ctx, dataset.PlaceListFilter{})
	if err != nil {
		t.Fatalf("ListPlaces() error = %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("list.Total = %d, want 1", list.Total)
	}

	server := NewWithDataset(nil, nil, nil, store, nil)
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "update_place_actions",
			"arguments": map[string]any{
				"placeKey": list.Results[0].PlaceKey,
				"actions": []map[string]any{
					{"type": "call", "status": "pending"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := newAuthorizedRequest(t, http.MethodPost, "/mcp", body)
	rec := httptest.NewRecorder()

	server.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("call")) {
		t.Fatalf("response does not include updated action: %s", rec.Body.String())
	}
}

func newAuthorizedRequest(t *testing.T, method, target string, body []byte) *http.Request {
	t.Helper()
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	return req
}

func stringPtrForMCPTest(value string) *string {
	return &value
}
