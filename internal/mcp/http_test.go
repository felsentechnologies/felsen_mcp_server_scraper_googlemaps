package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
	req := newAuthorizedRequest(t, http.MethodPost, "/mcp", body)
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

func TestHTTPBearerTokenRequired(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "")
	t.Setenv("MCP_BEARER_TOKEN", "secret-token")
	server := New(nil, nil, nil, nil)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
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
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
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
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
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
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
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

func newAuthorizedRequest(t *testing.T, method, target string, body []byte) *http.Request {
	t.Helper()
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	return req
}
