package httpauth

import (
	"net/http"
	"testing"
)

func TestServerBearerTokenPrefersHTTPToken(t *testing.T) {
	t.Setenv("HTTP_BEARER_TOKEN", "http-token")
	t.Setenv("MCP_BEARER_TOKEN", "mcp-token")

	if got := ServerBearerToken(); got != "http-token" {
		t.Fatalf("ServerBearerToken() = %q, want HTTP token", got)
	}
}

func TestValidBearerAuth(t *testing.T) {
	if !ValidBearerAuth("Bearer secret-token", "secret-token") {
		t.Fatal("ValidBearerAuth() = false, want true")
	}
	if ValidBearerAuth("Bearer wrong-token", "secret-token") {
		t.Fatal("ValidBearerAuth() = true for wrong token, want false")
	}
	if ValidBearerAuth("Basic secret-token", "secret-token") {
		t.Fatal("ValidBearerAuth() = true for wrong scheme, want false")
	}
}

func TestValidRequestAuthAcceptsAPIKeyHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-API-Key", "secret-token")
	if !ValidRequestAuth(headers, "secret-token") {
		t.Fatal("ValidRequestAuth(X-API-Key) = false, want true")
	}

	headers = http.Header{}
	headers.Set("Api-Key", "secret-token")
	if !ValidRequestAuth(headers, "secret-token") {
		t.Fatal("ValidRequestAuth(Api-Key) = false, want true")
	}

	headers = http.Header{}
	headers.Set("Authorization", "Bearer secret-token")
	if !ValidRequestAuth(headers, "secret-token") {
		t.Fatal("ValidRequestAuth(Authorization) = false, want true")
	}
}

func TestAllowedCORSOrigin(t *testing.T) {
	t.Setenv("MCP_ALLOWED_ORIGINS", "https://app.example.com")

	if got, ok := AllowedCORSOrigin("https://app.example.com", "api.example.com"); !ok || got != "https://app.example.com" {
		t.Fatalf("AllowedCORSOrigin(allowed) = %q, %v", got, ok)
	}
	if _, ok := AllowedCORSOrigin("https://evil.example.com", "api.example.com"); ok {
		t.Fatal("AllowedCORSOrigin(forbidden) ok = true, want false")
	}
	if got, ok := AllowedCORSOrigin("http://localhost:3000", "api.example.com"); !ok || got != "http://localhost:3000" {
		t.Fatalf("AllowedCORSOrigin(localhost) = %q, %v", got, ok)
	}
}

func TestAllowedCORSOriginAllowsOpenAIClientsByDefault(t *testing.T) {
	t.Setenv("MCP_ALLOWED_ORIGINS", "")

	for _, origin := range []string{
		"https://chatgpt.com",
		"https://chat.openai.com",
		"https://platform.openai.com",
		"https://developers.openai.com",
	} {
		if got, ok := AllowedCORSOrigin(origin, "googlemaps.mcp.technologies.felsen.enterprises"); !ok || got != origin {
			t.Fatalf("AllowedCORSOrigin(%q) = %q, %v; want allowed", origin, got, ok)
		}
	}
}
