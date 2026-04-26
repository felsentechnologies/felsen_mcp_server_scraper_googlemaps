package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"mcp_server_scraper_googlemaps/internal/dataset"
	"mcp_server_scraper_googlemaps/internal/extractors"
	"mcp_server_scraper_googlemaps/internal/httpauth"
	"mcp_server_scraper_googlemaps/internal/models"
	"mcp_server_scraper_googlemaps/internal/scraper"
)

type Server struct {
	in      io.Reader
	out     io.Writer
	scraper *scraper.Scraper
	dataset *dataset.Store
	logger  *log.Logger
}

func New(in io.Reader, out io.Writer, scraper *scraper.Scraper, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	var store *dataset.Store
	if scraper != nil {
		store = scraper.DatasetStore()
	}
	return &Server{in: in, out: out, scraper: scraper, dataset: store, logger: logger}
}

func NewWithDataset(in io.Reader, out io.Writer, scraper *scraper.Scraper, store *dataset.Store, logger *log.Logger) *Server {
	server := New(in, out, scraper, logger)
	server.dataset = store
	return server
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
		token := httpauth.ServerBearerToken()
		if token == "" {
			writeJSONRPC(w, http.StatusServiceUnavailable, errorResponse(nil, -32001, "server authentication is not configured"))
			return
		}
		if !httpauth.ValidBearerAuth(r.Header.Get("Authorization"), token) {
			w.Header().Set("WWW-Authenticate", httpauth.BearerRealm)
			writeJSONRPC(w, http.StatusUnauthorized, errorResponse(nil, -32001, "unauthorized"))
			return
		}
		if _, ok := httpauth.AllowedCORSOrigin(r.Header.Get("Origin"), r.Host); !ok {
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
		if s.scraper == nil {
			return toolError(req.ID, "scraper is not configured")
		}
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
	case "list_dataset_places":
		return s.listDatasetPlaces(ctx, req)
	case "list_pending_action_places":
		return s.listPendingActionPlaces(ctx, req)
	case "get_dataset_place":
		return s.getDatasetPlace(ctx, req)
	case "update_place_actions":
		return s.updatePlaceActions(ctx, req)
	case "append_place_action":
		return s.appendPlaceAction(ctx, req)
	default:
		return toolError(req.ID, fmt.Sprintf("unknown tool %q", params.Name))
	}
}

func (s *Server) listDatasetPlaces(ctx context.Context, req request) response {
	store, err := s.datasetStore()
	if err != nil {
		return toolError(req.ID, err.Error())
	}
	var filter dataset.PlaceListFilter
	if len(req.Params) > 0 {
		var params toolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return toolError(req.ID, "invalid list_dataset_places params")
		}
		if len(params.Arguments) > 0 {
			if err := json.Unmarshal(params.Arguments, &filter); err != nil {
				return toolError(req.ID, "invalid list_dataset_places arguments")
			}
		}
	}
	result, err := store.ListPlaces(ctx, filter)
	if err != nil {
		return toolError(req.ID, err.Error())
	}
	return toolJSON(req.ID, result)
}

func (s *Server) listPendingActionPlaces(ctx context.Context, req request) response {
	store, err := s.datasetStore()
	if err != nil {
		return toolError(req.ID, err.Error())
	}
	var filter dataset.PlaceListFilter
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return toolError(req.ID, "invalid list_pending_action_places params")
	}
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &filter); err != nil {
			return toolError(req.ID, "invalid list_pending_action_places arguments")
		}
	}
	if filter.MissingActionType == "" {
		filter.PendingActions = true
	}
	result, err := store.ListPlaces(ctx, filter)
	if err != nil {
		return toolError(req.ID, err.Error())
	}
	return toolJSON(req.ID, result)
}

func (s *Server) getDatasetPlace(ctx context.Context, req request) response {
	store, err := s.datasetStore()
	if err != nil {
		return toolError(req.ID, err.Error())
	}
	var args struct {
		ID       int64  `json:"id,omitempty"`
		PlaceKey string `json:"placeKey,omitempty"`
	}
	if err := unmarshalToolArguments(req.Params, &args); err != nil {
		return toolError(req.ID, "invalid get_dataset_place arguments")
	}
	place, err := store.GetPlace(ctx, args.ID, args.PlaceKey)
	if err != nil {
		return toolError(req.ID, err.Error())
	}
	return toolJSON(req.ID, place)
}

func (s *Server) updatePlaceActions(ctx context.Context, req request) response {
	store, err := s.datasetStore()
	if err != nil {
		return toolError(req.ID, err.Error())
	}
	var args struct {
		ID       int64               `json:"id,omitempty"`
		PlaceKey string              `json:"placeKey,omitempty"`
		Actions  []models.ActionData `json:"actions"`
	}
	if err := unmarshalToolArguments(req.Params, &args); err != nil {
		return toolError(req.ID, "invalid update_place_actions arguments")
	}
	place, err := store.UpdatePlaceActions(ctx, args.ID, args.PlaceKey, args.Actions)
	if err != nil {
		return toolError(req.ID, err.Error())
	}
	return toolJSON(req.ID, place)
}

func (s *Server) appendPlaceAction(ctx context.Context, req request) response {
	store, err := s.datasetStore()
	if err != nil {
		return toolError(req.ID, err.Error())
	}
	var args struct {
		ID       int64             `json:"id,omitempty"`
		PlaceKey string            `json:"placeKey,omitempty"`
		Action   models.ActionData `json:"action"`
	}
	if err := unmarshalToolArguments(req.Params, &args); err != nil {
		return toolError(req.ID, "invalid append_place_action arguments")
	}
	place, err := store.AppendPlaceAction(ctx, args.ID, args.PlaceKey, args.Action)
	if err != nil {
		return toolError(req.ID, err.Error())
	}
	return toolJSON(req.ID, place)
}

func (s *Server) datasetStore() (*dataset.Store, error) {
	if s.dataset == nil {
		return nil, dataset.ErrDatasetUnavailable
	}
	return s.dataset, nil
}

func unmarshalToolArguments(params json.RawMessage, target any) error {
	var call toolCallParams
	if err := json.Unmarshal(params, &call); err != nil {
		return err
	}
	if len(call.Arguments) == 0 {
		call.Arguments = []byte("{}")
	}
	return json.Unmarshal(call.Arguments, target)
}

func tools() []map[string]any {
	return []map[string]any{
		{
			"name":        "scrape_google_maps",
			"title":       "Scrape Google Maps",
			"description": "Search Google Maps and extract place data plus emails, phones, social links and optional reviews.",
			"annotations": toolAnnotations(false, false, false, true),
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"searchQueries": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Google Maps search queries.",
					},
					"maxPlacesPerQuery": map[string]any{"type": "integer", "default": 20, "minimum": 1, "maximum": 500},
					"scrapeEmails":      map[string]any{"type": "boolean", "default": true},
					"scrapePhones":      map[string]any{"type": "boolean", "default": true},
					"scrapeReviews":     map[string]any{"type": "boolean", "default": false},
					"maxReviewsPerPlace": map[string]any{
						"type":    "integer",
						"default": 10,
						"minimum": 0,
						"maximum": 100,
					},
					"language": map[string]any{"type": "string", "default": "pt-BR"},
					"proxyConfiguration": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]any{
							"proxyUrls": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
					},
				},
				"required": []string{"searchQueries"},
			},
		},
		{
			"name":        "list_dataset_places",
			"title":       "List Dataset Places",
			"description": "List persisted dataset_places records with pagination and filters for query, category, rating, reviews and actions.",
			"annotations": toolAnnotations(true, false, true, false),
			"inputSchema": datasetPlaceFilterSchema(),
		},
		{
			"name":        "list_pending_action_places",
			"title":       "List Pending Action Places",
			"description": "List persisted places that have no actions, or places missing a specific action type when missingActionType is provided.",
			"annotations": toolAnnotations(true, false, true, false),
			"inputSchema": datasetPlaceFilterSchema(),
		},
		{
			"name":        "get_dataset_place",
			"title":       "Get Dataset Place",
			"description": "Get one persisted dataset place by id or placeKey.",
			"annotations": toolAnnotations(true, false, true, false),
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"id":       map[string]any{"type": "integer"},
					"placeKey": map[string]any{"type": "string"},
				},
			},
		},
		{
			"name":        "update_place_actions",
			"title":       "Update Place Actions",
			"description": "Replace dataset_places.actions for one place. Actions must be a JSON array of objects.",
			"annotations": toolAnnotations(false, true, true, false),
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"id":       map[string]any{"type": "integer"},
					"placeKey": map[string]any{"type": "string"},
					"actions": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "object"},
					},
				},
				"required": []string{"actions"},
			},
		},
		{
			"name":        "append_place_action",
			"title":       "Append Place Action",
			"description": "Append one JSON object to dataset_places.actions without replacing existing actions.",
			"annotations": toolAnnotations(false, false, false, false),
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"id":       map[string]any{"type": "integer"},
					"placeKey": map[string]any{"type": "string"},
					"action":   map[string]any{"type": "object"},
				},
				"required": []string{"action"},
			},
		},
		{
			"name":        "extract_contacts_from_html",
			"title":       "Extract Contacts From HTML",
			"description": "Extract emails, phones, social links and optional contact page URLs from a raw HTML string.",
			"annotations": toolAnnotations(true, false, true, false),
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"html":    map[string]any{"type": "string"},
					"baseUrl": map[string]any{"type": "string"},
				},
				"required": []string{"html"},
			},
		},
	}
}

func toolAnnotations(readOnly, destructive, idempotent, openWorld bool) map[string]any {
	return map[string]any{
		"readOnlyHint":    readOnly,
		"destructiveHint": destructive,
		"idempotentHint":  idempotent,
		"openWorldHint":   openWorld,
	}
}

func datasetPlaceFilterSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"limit":             map[string]any{"type": "integer", "default": dataset.DefaultPlaceListLimit, "minimum": 1, "maximum": dataset.MaxPlaceListLimit},
			"offset":            map[string]any{"type": "integer", "default": 0, "minimum": 0},
			"query":             map[string]any{"type": "string"},
			"search":            map[string]any{"type": "string"},
			"category":          map[string]any{"type": "string"},
			"placeKey":          map[string]any{"type": "string"},
			"minRating":         map[string]any{"type": "number"},
			"maxRating":         map[string]any{"type": "number"},
			"hasReviews":        map[string]any{"type": "boolean"},
			"pendingActions":    map[string]any{"type": "boolean"},
			"actionType":        map[string]any{"type": "string"},
			"missingActionType": map[string]any{"type": "string"},
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
