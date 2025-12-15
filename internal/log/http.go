package log

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// NewHTTPClient creates an HTTP client with debug logging enabled when debug mode is on.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: &HTTPRoundTripLogger{
			Transport: http.DefaultTransport,
		},
	}
}

// HTTPRoundTripLogger is an http.RoundTripper that logs requests and responses.
type HTTPRoundTripLogger struct {
	Transport http.RoundTripper
}

const maxLoggedBodyBytes = 64 * 1024

// RoundTrip implements http.RoundTripper interface with logging.
func (h *HTTPRoundTripLogger) RoundTrip(req *http.Request) (*http.Response, error) {
	var err error
	var save io.ReadCloser
	save, req.Body, err = drainBody(req.Body)
	if err != nil {
		slog.Error(
			"HTTP request failed",
			"method", req.Method,
			"url", req.URL,
			"error", err,
		)
		return nil, err
	}

	if slog.Default().Enabled(req.Context(), slog.LevelDebug) {
		slog.Debug(
			"HTTP Request",
			"method", req.Method,
			"url", req.URL,
			"body", bodyToStringLimited(save, maxLoggedBodyBytes),
		)
	}

	start := time.Now()
	resp, err := h.Transport.RoundTrip(req)
	duration := time.Since(start)
	if err != nil {
		slog.Error(
			"HTTP request failed",
			"method", req.Method,
			"url", req.URL,
			"duration_ms", duration.Milliseconds(),
			"error", err,
		)
		return resp, err
	}

	if slog.Default().Enabled(req.Context(), slog.LevelDebug) {
		// Do not drain streaming bodies; draining blocks until completion and
		// defeats incremental streaming updates.
		if isStreamingContentType(resp.Header.Get("Content-Type")) {
			slog.Debug(
				"HTTP Response",
				"status_code", resp.StatusCode,
				"status", resp.Status,
				"headers", formatHeaders(resp.Header),
				"body", "[streaming body omitted]",
				"content_length", resp.ContentLength,
				"duration_ms", duration.Milliseconds(),
			)
			return resp, nil
		}

		save, resp.Body, err = drainBody(resp.Body)
		slog.Debug(
			"HTTP Response",
			"status_code", resp.StatusCode,
			"status", resp.Status,
			"headers", formatHeaders(resp.Header),
			"body", bodyToStringLimited(save, maxLoggedBodyBytes),
			"content_length", resp.ContentLength,
			"duration_ms", duration.Milliseconds(),
			"error", err,
		)
	}
	return resp, err
}

func isStreamingContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	// Streaming protocols used by providers (SSE / NDJSON).
	return strings.Contains(ct, "text/event-stream") ||
		strings.Contains(ct, "application/x-ndjson")
}

func bodyToStringLimited(body io.ReadCloser, maxBytes int) string {
	if body == nil {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = maxLoggedBodyBytes
	}
	src, err := io.ReadAll(io.LimitReader(body, int64(maxBytes+1)))
	if err != nil {
		slog.Error("Failed to read body", "error", err)
		return ""
	}
	truncated := len(src) > maxBytes
	if truncated {
		src = src[:maxBytes]
	}

	var b bytes.Buffer
	trimmed := bytes.TrimSpace(src)
	// Avoid indenting large payloads (expensive); only pretty-print small JSON.
	if len(trimmed) <= 8*1024 && json.Indent(&b, trimmed, "", "  ") == nil {
		if truncated {
			return b.String() + "\n…[truncated]"
		}
		return b.String()
	}

	if json.Indent(&b, trimmed, "", "  ") != nil {
		// not json probably
		if truncated {
			return string(src) + "\n…[truncated]"
		}
		return string(src)
	}
	if truncated {
		return b.String() + "\n…[truncated]"
	}
	return b.String()
}

// formatHeaders formats HTTP headers for logging, filtering out sensitive information.
func formatHeaders(headers http.Header) map[string][]string {
	filtered := make(map[string][]string)
	for key, values := range headers {
		lowerKey := strings.ToLower(key)
		// Filter out sensitive headers
		if strings.Contains(lowerKey, "authorization") ||
			strings.Contains(lowerKey, "api-key") ||
			strings.Contains(lowerKey, "token") ||
			strings.Contains(lowerKey, "secret") {
			filtered[key] = []string{"[REDACTED]"}
		} else {
			filtered[key] = values
		}
	}
	return filtered
}

func drainBody(b io.ReadCloser) (r1, r2 io.ReadCloser, err error) {
	if b == nil || b == http.NoBody {
		return http.NoBody, http.NoBody, nil
	}
	var buf bytes.Buffer
	if _, err = buf.ReadFrom(b); err != nil {
		return nil, b, err
	}
	if err = b.Close(); err != nil {
		return nil, b, err
	}
	return io.NopCloser(&buf), io.NopCloser(bytes.NewReader(buf.Bytes())), nil
}
