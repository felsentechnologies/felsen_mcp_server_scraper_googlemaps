package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"mcp_server_scraper_googlemaps/internal/mcp"
	"mcp_server_scraper_googlemaps/internal/models"
	"mcp_server_scraper_googlemaps/internal/scraper"
)

type Server struct {
	scraper *scraper.Scraper
	logger  *log.Logger
}

func New(scraper *scraper.Scraper, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{scraper: scraper, logger: logger}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/scrape", s.scrape)
	mux.Handle("/mcp", mcp.New(nil, nil, s.scraper, s.logger).HTTPHandler())
	return withCORS(withSecurityGateway(mux))
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	server := &http.Server{Addr: addr, Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	s.logger.Printf("HTTP server listening on %s", addr)
	return server.ListenAndServe()
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) scrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var input models.Input
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if len(input.SearchQueries) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "searchQueries is required and must not be empty"})
		return
	}

	results, err := s.scraper.ScrapeGoogleMaps(r.Context(), input)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		if errors.Is(err, context.Canceled) {
			status = 499
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"count":   len(results),
		"results": results,
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		allowedOrigin, ok := allowedCORSOrigin(origin, r.Host)
		if origin != "" && !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden origin"})
			return
		}
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, MCP-Protocol-Version, Mcp-Session-Id, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withSecurityGateway(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		token := serverBearerToken()
		if token == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "server authentication is not configured",
			})
			return
		}

		if !validBearerAuth(r.Header.Get("Authorization"), token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="mcp-googlemaps"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func serverBearerToken() string {
	if value := strings.TrimSpace(os.Getenv("HTTP_BEARER_TOKEN")); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("MCP_BEARER_TOKEN"))
}

func validBearerAuth(headerValue, expectedToken string) bool {
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

func allowedCORSOrigin(origin, host string) (string, bool) {
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
	originHost = hostnameOnly(originHost)
	host = hostnameOnly(host)

	if strings.EqualFold(originHost, host) || isLocalhost(originHost) {
		return origin, true
	}
	return "", false
}

func hostnameOnly(host string) string {
	name, _, err := net.SplitHostPort(host)
	if err == nil {
		return name
	}
	return strings.Trim(host, "[]")
}

func isLocalhost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
