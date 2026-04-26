package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"mcp_server_scraper_googlemaps/internal/httpauth"
	"mcp_server_scraper_googlemaps/internal/mcp"
	"mcp_server_scraper_googlemaps/internal/models"
	"mcp_server_scraper_googlemaps/internal/scraper"
)

const maxScrapeBodyBytes = 1 << 20

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
	server := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
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
	r.Body = http.MaxBytesReader(w, r.Body, maxScrapeBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if len(input.SearchQueries) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "searchQueries is required and must not be empty"})
		return
	}
	if s.scraper == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scraper is not configured"})
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
		allowedOrigin, ok := httpauth.AllowedCORSOrigin(origin, r.Host)
		if origin != "" && !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden origin"})
			return
		}
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, MCP-Protocol-Version, Mcp-Session-Id, Authorization, X-API-Key, Api-Key")
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
		if r.URL.Path == "/mcp" {
			next.ServeHTTP(w, r)
			return
		}

		token := httpauth.ServerBearerToken()
		if token == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "server authentication is not configured",
			})
			return
		}

		if !httpauth.ValidRequestAuth(r.Header, token) {
			w.Header().Set("WWW-Authenticate", httpauth.BearerRealm)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
