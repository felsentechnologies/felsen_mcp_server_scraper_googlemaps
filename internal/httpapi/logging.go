package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultHTTPLogBodyLimit = 2 << 20

type loggingResponseWriter struct {
	http.ResponseWriter
	status    int
	bytes     int64
	body      bytes.Buffer
	limit     int
	truncated bool
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.limit > 0 && w.body.Len() < w.limit {
		remaining := w.limit - w.body.Len()
		if len(p) > remaining {
			_, _ = w.body.Write(p[:remaining])
			w.truncated = true
		} else {
			_, _ = w.body.Write(p)
		}
	} else if len(p) > 0 {
		w.truncated = true
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

func (w *loggingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func withHTTPLogging(next http.Handler, logger *log.Logger) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		limit := httpLogBodyLimit()
		requestBody, requestBodyTruncated, requestBodyReadErr := captureRequestBody(r, limit)
		id := requestID(r)
		requestInfo := map[string]any{
			"event":                   "http_request_received",
			"request_id":              id,
			"method":                  r.Method,
			"path":                    r.URL.Path,
			"raw_query":               r.URL.RawQuery,
			"host":                    r.Host,
			"remote_addr":             r.RemoteAddr,
			"client_ip":               clientIP(r),
			"forwarded_for":           r.Header.Get("X-Forwarded-For"),
			"forwarded_host":          r.Header.Get("X-Forwarded-Host"),
			"forwarded_proto":         r.Header.Get("X-Forwarded-Proto"),
			"real_ip":                 r.Header.Get("X-Real-IP"),
			"origin":                  r.Header.Get("Origin"),
			"referer":                 r.Header.Get("Referer"),
			"user_agent":              r.UserAgent(),
			"content_length":          r.ContentLength,
			"headers":                 redactedHeaders(r.Header),
			"body":                    requestBody,
			"body_truncated":          requestBodyTruncated,
			"mcp_protocol_version":    r.Header.Get("MCP-Protocol-Version"),
			"mcp_session_id":          r.Header.Get("Mcp-Session-Id"),
			"jsonrpc_method":          jsonRPCMethod(requestBody),
			"jsonrpc_tool_name":       jsonRPCToolName(requestBody),
			"request_body_read_error": errorString(requestBodyReadErr),
		}
		logJSON(logger, requestInfo)

		rec := &loggingResponseWriter{ResponseWriter: w, limit: limit}
		next.ServeHTTP(rec, r)

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		responseInfo := map[string]any{
			"event":              "http_response_sent",
			"request_id":         id,
			"method":             r.Method,
			"path":               r.URL.Path,
			"status":             status,
			"duration_ms":        time.Since(start).Milliseconds(),
			"bytes_written":      rec.bytes,
			"headers":            redactedHeaders(rec.Header()),
			"body":               rec.body.String(),
			"body_truncated":     rec.truncated,
			"jsonrpc_method":     jsonRPCMethod(requestBody),
			"jsonrpc_tool_name":  jsonRPCToolName(requestBody),
			"response_body_size": rec.body.Len(),
		}
		logJSON(logger, responseInfo)
	})
}

func captureRequestBody(r *http.Request, limit int) (string, bool, error) {
	if r.Body == nil {
		return "", false, nil
	}
	prefix, err := io.ReadAll(io.LimitReader(r.Body, int64(limit)+1))
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(prefix), r.Body))
	if err != nil {
		return "", false, err
	}
	if len(prefix) > limit {
		return string(prefix[:limit]), true, nil
	}
	return string(prefix), false, nil
}

func httpLogBodyLimit() int {
	value := strings.TrimSpace(os.Getenv("HTTP_DEBUG_LOG_BODY_LIMIT"))
	if value == "" {
		return defaultHTTPLogBodyLimit
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 0 {
		return defaultHTTPLogBodyLimit
	}
	return limit
}

func requestID(r *http.Request) string {
	for _, name := range []string{"X-Request-Id", "X-Request-ID", "Cf-Ray", "Fly-Request-Id"} {
		if value := strings.TrimSpace(r.Header.Get(name)); value != "" {
			return value
		}
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func clientIP(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); value != "" {
		parts := strings.Split(value, ",")
		return strings.TrimSpace(parts[0])
	}
	if value := strings.TrimSpace(r.Header.Get("X-Real-IP")); value != "" {
		return value
	}
	return r.RemoteAddr
}

func redactedHeaders(headers http.Header) map[string][]string {
	out := make(map[string][]string, len(headers))
	for name, values := range headers {
		if isSecretHeader(name) {
			out[name] = []string{"[REDACTED]"}
			continue
		}
		copied := make([]string, len(values))
		copy(copied, values)
		out[name] = copied
	}
	return out
}

func isSecretHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "x-api-key", "api-key", "cookie", "set-cookie":
		return true
	default:
		return false
	}
}

func jsonRPCMethod(body string) string {
	var req struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		return ""
	}
	return req.Method
}

func jsonRPCToolName(body string) string {
	var req struct {
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		return ""
	}
	return req.Params.Name
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func logJSON(logger *log.Logger, value map[string]any) {
	payload, err := json.Marshal(value)
	if err != nil {
		logger.Printf("http_log_marshal_error=%q", err.Error())
		return
	}
	logger.Print(string(payload))
}
