package httpauth

import (
	"crypto/subtle"
	"net"
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

func AllowedCORSOrigin(origin, host string) (string, bool) {
	if origin == "" {
		return "", true
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
