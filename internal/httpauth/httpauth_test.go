package httpauth

import "testing"

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
