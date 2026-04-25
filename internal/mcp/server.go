package mcp

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"mcp_server_scraper_googlemaps/internal/extractors"
	"mcp_server_scraper_googlemaps/internal/models"
	"mcp_server_scraper_googlemaps/internal/scraper"
)

type Server struct {
	in      io.Reader
	out     io.Writer
	scraper *scraper.Scraper
	logger  *log.Logger
}

func New(in io.Reader, out io.Writer, scraper *scraper.Scraper, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{in: in, out: out, scraper: scraper, logger: logger}
}

func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 1024), 20*1024*1024)
	encoder := json.NewEncoder(s.out)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = encoder.Encode(errorResponse(nil, -32700, "parse error"))
			continue
		}

		resp, ok := s.handle(ctx, req)
		if !ok {
			continue
		}
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validBearerToken(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="mcp"`)
			writeJSONRPC(w, http.StatusUnauthorized, errorResponse(nil, -32001, "unauthorized"))
			return
		}
		if !validOrigin(r) {
			writeJSONRPC(w, http.StatusForbidden, errorResponse(nil, -32000, "forbidden origin"))
			return
		}
		if !validProtocolVersion(r.Header.Get("MCP-Protocol-Version")) {
			writeJSONRPC(w, http.StatusBadRequest, errorResponse(nil, -32000, "unsupported MCP protocol version"))
			return
		}

		switch r.Method {
		case http.MethodPost:
			s.handleHTTPPost(w, r)
		case http.MethodGet:
			w.Header().Set("Allow", "POST")
			writeJSONRPC(w, http.StatusMethodNotAllowed, errorResponse(nil, -32000, "SSE stream is not supported by this server"))
		case http.MethodDelete:
			w.WriteHeader(http.StatusMethodNotAllowed)
		default:
			w.Header().Set("Allow", "POST")
			writeJSONRPC(w, http.StatusMethodNotAllowed, errorResponse(nil, -32600, "method not allowed"))
		}
	})
}

func (s *Server) handleHTTPPost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 20*1024*1024))
	if err != nil {
		writeJSONRPC(w, http.StatusBadRequest, errorResponse(nil, -32700, "parse error"))
		return
	}

	bodyText := strings.TrimSpace(string(body))
	if bodyText == "" {
		writeJSONRPC(w, http.StatusBadRequest, errorResponse(nil, -32600, "empty request body"))
		return
	}

	if strings.HasPrefix(bodyText, "[") {
		s.handleHTTPBatch(w, r, body)
		return
	}

	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONRPC(w, http.StatusBadRequest, errorResponse(nil, -32700, "parse error"))
		return
	}
	if req.Method == "" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	resp, ok := s.handle(r.Context(), req)
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSONRPC(w, http.StatusOK, resp)
}

func (s *Server) handleHTTPBatch(w http.ResponseWriter, r *http.Request, body []byte) {
	var reqs []request
	if err := json.Unmarshal(body, &reqs); err != nil {
		writeJSONRPC(w, http.StatusBadRequest, errorResponse(nil, -32700, "parse error"))
		return
	}
	if len(reqs) == 0 {
		writeJSONRPC(w, http.StatusBadRequest, errorResponse(nil, -32600, "empty batch"))
		return
	}

	responses := make([]response, 0, len(reqs))
	for _, req := range reqs {
		if req.Method == "" {
			continue
		}
		resp, ok := s.handle(r.Context(), req)
		if ok {
			responses = append(responses, resp)
		}
	}
	if len(responses) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSONRPC(w, http.StatusOK, responses)
}

func (s *Server) handle(ctx context.Context, req request) (response, bool) {
	switch req.Method {
	case "initialize":
		protocolVersion := negotiatedProtocolVersion(req.Params)
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "mcp-server-scraper-googlemaps",
				"version": "1.0.0",
			},
		}}, true
	case "notifications/initialized":
		return response{}, false
	case "ping":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}, true
	case "tools/list":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": tools()}}, true
	case "tools/call":
		return s.callTool(ctx, req), true
	case "resources/list":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"resources": []any{}}}, true
	case "prompts/list":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"prompts": []any{}}}, true
	default:
		if req.ID == nil {
			return response{}, false
		}
		return errorResponse(req.ID, -32601, "method not found"), true
	}
}

func (s *Server) callTool(ctx context.Context, req request) response {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid tool call params")
	}

	switch params.Name {
	case "scrape_google_maps":
		var input models.Input
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return toolError(req.ID, "invalid scrape_google_maps arguments")
		}
		results, err := s.scraper.ScrapeGoogleMaps(ctx, input)
		if err != nil {
			return toolError(req.ID, err.Error())
		}
		return toolJSON(req.ID, map[string]any{"count": len(results), "results": results})
	case "extract_contacts_from_html":
		var args struct {
			HTML    string `json:"html"`
			BaseURL string `json:"baseUrl,omitempty"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return toolError(req.ID, "invalid extract_contacts_from_html arguments")
		}
		result := map[string]any{
			"emails":      extractors.ExtractEmails(args.HTML),
			"phones":      extractors.ExtractPhones(args.HTML),
			"socialLinks": extractors.ExtractSocialLinks(args.HTML),
		}
		if args.BaseURL != "" {
			result["contactPageUrls"] = extractors.FindContactPageURLs(args.HTML, args.BaseURL)
		}
		return toolJSON(req.ID, result)
	default:
		return toolError(req.ID, fmt.Sprintf("unknown tool %q", params.Name))
	}
}

func tools() []map[string]any {
	return []map[string]any{
		{
			"name":        "scrape_google_maps",
			"description": "Search Google Maps and extract place data plus emails, phones and social links from business websites.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"searchQueries": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Google Maps search queries.",
					},
					"maxPlacesPerQuery": map[string]any{"type": "integer", "default": 20, "minimum": 1, "maximum": 500},
					"scrapeEmails":      map[string]any{"type": "boolean", "default": true},
					"scrapePhones":      map[string]any{"type": "boolean", "default": true},
					"language":          map[string]any{"type": "string", "default": "pt-BR"},
					"proxyConfiguration": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"proxyUrls": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
					},
				},
				"required": []string{"searchQueries"},
			},
		},
		{
			"name":        "extract_contacts_from_html",
			"description": "Extract emails, phones, social links and optional contact page URLs from a raw HTML string.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"html":    map[string]any{"type": "string"},
					"baseUrl": map[string]any{"type": "string"},
				},
				"required": []string{"html"},
			},
		},
	}
}

func toolJSON(id *json.RawMessage, value any) response {
	payload, _ := json.MarshalIndent(value, "", "  ")
	return response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []map[string]string{{"type": "text", "text": string(payload)}},
	}}
}

func toolError(id *json.RawMessage, message string) response {
	return response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"isError": true,
		"content": []map[string]string{{"type": "text", "text": message}},
	}}
}

func errorResponse(id *json.RawMessage, code int, message string) response {
	return response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

func writeJSONRPC(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func negotiatedProtocolVersion(params json.RawMessage) string {
	var initParams struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 && json.Unmarshal(params, &initParams) == nil && isSupportedProtocolVersion(initParams.ProtocolVersion) {
		return initParams.ProtocolVersion
	}
	return latestProtocolVersion
}

func validProtocolVersion(version string) bool {
	return version == "" || isSupportedProtocolVersion(version)
}

func isSupportedProtocolVersion(version string) bool {
	switch version {
	case "2024-11-05", "2025-03-26", latestProtocolVersion:
		return true
	default:
		return false
	}
}

func validOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	for _, allowed := range strings.Split(os.Getenv("MCP_ALLOWED_ORIGINS"), ",") {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" || strings.EqualFold(allowed, origin) {
			return true
		}
	}

	originHost := origin
	if strings.Contains(origin, "://") {
		parsedOrigin, err := http.NewRequest(http.MethodGet, origin, nil)
		if err == nil && parsedOrigin.URL.Host != "" {
			originHost = parsedOrigin.URL.Host
		}
	}
	host := r.Host
	originName, _, originErr := net.SplitHostPort(originHost)
	hostName, _, hostErr := net.SplitHostPort(host)
	if originErr == nil {
		originHost = originName
	}
	if hostErr == nil {
		host = hostName
	}

	return strings.EqualFold(originHost, host) || isLocalhost(originHost)
}

func isLocalhost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func validBearerToken(r *http.Request) bool {
	expected := strings.TrimSpace(os.Getenv("HTTP_BEARER_TOKEN"))
	if expected == "" {
		expected = strings.TrimSpace(os.Getenv("MCP_BEARER_TOKEN"))
	}
	if expected == "" {
		return true
	}

	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return false
	}

	scheme, token, ok := strings.Cut(auth, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

const latestProtocolVersion = "2025-06-18"

type request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type response struct {
	JSONRPC string           `json:"jsonrpc,omitempty"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
