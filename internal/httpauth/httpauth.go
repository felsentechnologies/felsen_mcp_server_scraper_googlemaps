package httpauth

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const BearerRealm = `Bearer realm="mcp-googlemaps"`

func ServerBearerToken() string {
	if value := strings.TrimSpace(os.Getenv("HTTP_BEARER_TOKEN")); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("MCP_BEARER_TOKEN"))
}

func ValidBearerAuth(headerValue, expectedToken string) bool {
	headerValue = strings.TrimSpace(headerValue)
	if headerValue == "" || expectedToken == "" {
		return false
	}

	scheme, token, ok := strings.Cut(headerValue, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) == 1
}

func ValidAPIKey(value, expectedToken string) bool {
	value = strings.TrimSpace(value)
	if value == "" || expectedToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(value), []byte(expectedToken)) == 1
}

func ValidRequestAuth(headers http.Header, expectedToken string) bool {
	if ValidBearerAuth(headers.Get("Authorization"), expectedToken) {
		return true
	}
	if ValidAPIKey(headers.Get("X-API-Key"), expectedToken) {
		return true
	}
	return ValidAPIKey(headers.Get("Api-Key"), expectedToken)
}

func AllowedCORSOrigin(origin, host string) (string, bool) {
	if origin == "" {
		return "", true
	}

	if isDefaultAllowedOrigin(origin) {
		return origin, true
	}

	for _, allowed := range strings.Split(os.Getenv("MCP_ALLOWED_ORIGINS"), ",") {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" {
			return "*", true
		}
		if strings.EqualFold(allowed, origin) {
			return origin, true
		}
	}

	originHost := origin
	if parsed, err := url.Parse(origin); err == nil && parsed.Host != "" {
		originHost = parsed.Host
	}
	originHost = HostnameOnly(originHost)
	host = HostnameOnly(host)

	if strings.EqualFold(originHost, host) || IsLocalhost(originHost) {
		return origin, true
	}
	return "", false
}

func isDefaultAllowedOrigin(origin string) bool {
	switch strings.ToLower(strings.TrimRight(strings.TrimSpace(origin), "/")) {
	case "https://chatgpt.com",
		"https://chat.openai.com",
		"https://platform.openai.com",
		"https://developers.openai.com":
		return true
	default:
		return false
	}
}

func IsLocalhost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func HostnameOnly(host string) string {
	name, _, err := net.SplitHostPort(host)
	if err == nil {
		return name
	}
	return strings.Trim(host, "[]")
}
