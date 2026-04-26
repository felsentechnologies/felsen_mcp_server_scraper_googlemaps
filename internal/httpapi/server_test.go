package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGatewayRequiresConfiguredToken(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "")
	t.Setenv("MCP_BEARER_TOKEN", "")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestGatewayRejectsMissingToken(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("WWW-Authenticate header is empty")
	}
}

func TestGatewayRejectsInvalidToken(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestGatewayAllowsValidTokenForHealth(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGatewayProtectsScrape(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")

	req := httptest.NewRequest(http.MethodPost, "/scrape", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestGatewayAllowsMCPDiscoveryWithoutToken(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGatewayAllowsMCPDiscoveryWithTrailingSlash(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp/", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestGatewayProtectsMCPToolCallsWithoutToken(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")
	t.Setenv("MCP_REQUIRE_TOOL_AUTH", "true")

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"extract_contacts_from_html","arguments":{"html":"<html></html>"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestGatewayAllowsValidTokenForMCP(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")

	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestScrapeRejectsLargeBody(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")

	body := `{"searchQueries":["` + strings.Repeat("a", maxScrapeBodyBytes) + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/scrape", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
}

func TestScrapeWithNilScraperReturnsServerError(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")

	body := []byte(`{"searchQueries":["pizza"],"maxPlacesPerQuery":1}`)
	req := httptest.NewRequest(http.MethodPost, "/scrape", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "scraper is not configured") {
		t.Fatalf("response = %s, want nil scraper error", rec.Body.String())
	}
}

func TestGatewayAllowsCORSPreflightWithoutToken(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")
	t.Setenv("MCP_ALLOWED_ORIGINS", "https://app.example.com")

	req := httptest.NewRequest(http.MethodOptions, "/scrape", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want allowed origin", got)
	}
}

func TestGatewayRejectsForbiddenCORSOrigin(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "secret-token")
	t.Setenv("MCP_BEARER_TOKEN", "")
	t.Setenv("MCP_ALLOWED_ORIGINS", "https://app.example.com")

	req := httptest.NewRequest(http.MethodOptions, "/scrape", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()

	New(nil, nil).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
