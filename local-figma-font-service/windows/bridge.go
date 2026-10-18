// Package fontservice relays the Figma desktop font API without reading font files.
package fontservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	listTarget         = "/figma/font-files?freetype_minimum_api_version=20&isolate=true&"
	filePrefix         = "/figma/font-file?file="
	upstreamBase       = "http://localhost:44950"
	listLimit    int64 = 8_000_000
	fontLimit    int64 = 20_000_000
)

type bridge struct {
	client  *http.Client
	busy    chan struct{}
	mu      sync.RWMutex
	allowed map[string]struct{}
	health  []byte
}

// NewHandler returns a fresh relay with an empty catalog allowlist.
func NewHandler() http.Handler {
	return newHandler(&http.Transport{
		Proxy:                  nil,
		DialContext:            (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		DisableKeepAlives:      true,
		DisableCompression:     true,
		MaxResponseHeaderBytes: 8192,
		ResponseHeaderTimeout:  12 * time.Second,
	})
}

// Transport injection is private and used only by tests.
func newHandler(transport http.RoundTripper) *bridge {
	var id [16]byte
	_, _ = rand.Read(id[:])
	health, _ := json.Marshal(map[string]any{
		"service": "figma-local-font-service", "version": 1,
		"activationKind": "login-autostart", "idleExitSeconds": 0,
		"pid": os.Getpid(), "sessionId": hex.EncodeToString(id[:]),
		"startedAt": time.Now().UTC().Format(time.RFC3339Nano), "fontRelay": true,
	})
	return &bridge{client: &http.Client{Transport: transport, Timeout: 12 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		busy: make(chan struct{}, 1), allowed: map[string]struct{}{}, health: health}
}

func reply(w http.ResponseWriter, status int, body []byte, binary, preflight bool) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	if binary {
		h.Set("Content-Type", "application/octet-stream")
	}
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Access-Control-Allow-Origin", "null")
	h.Set("Vary", "Origin")
	h.Set("Connection", "close")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	if preflight {
		h.Set("Access-Control-Allow-Methods", "GET")
		h.Set("Access-Control-Max-Age", "600")
		h.Set("Access-Control-Allow-Private-Network", "true")
	}
	// The send budget starts after upstream work, independently of its timeout.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (b *bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	deny := func(status int) { reply(w, status, nil, false, false) }
	if r.ProtoMajor != 1 || r.ProtoMinor != 1 || r.URL == nil || r.URL.IsAbs() || r.URL.Host != "" || r.URL.Fragment != "" {
		deny(http.StatusBadRequest)
		return
	}
	for _, name := range []string{"Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers", "Content-Length", "Host"} {
		if len(r.Header.Values(name)) > 1 {
			deny(http.StatusBadRequest)
			return
		}
	}
	if len(r.TransferEncoding) > 0 || len(r.Header.Values("Transfer-Encoding")) > 0 || r.ContentLength != 0 {
		deny(http.StatusBadRequest)
		return
	}
	if v, ok := r.Header["Content-Length"]; ok && (len(v) != 1 || v[0] != "0") {
		deny(http.StatusBadRequest)
		return
	}
	if r.Host != "localhost:38474" || (len(r.Header.Values("Origin")) != 0 && r.Header.Get("Origin") != "null") {
		deny(http.StatusForbidden)
		return
	}
	target := r.RequestURI
	list, health := target == listTarget, target == "/health"
	isFile := strings.HasPrefix(target, filePrefix) && !strings.ContainsAny(target, "&#")
	var file string
	if isFile {
		var err error
		file, err = url.QueryUnescape(strings.TrimPrefix(target, filePrefix))
		if err != nil || !utf8.ValidString(file) {
			deny(http.StatusBadRequest)
			return
		}
	}
	if !list && !health && !isFile {
		deny(http.StatusNotFound)
		return
	}
	if r.Method == http.MethodOptions {
		if r.Header.Get("Access-Control-Request-Method") != "GET" || r.Header.Get("Access-Control-Request-Headers") != "" {
			deny(http.StatusForbidden)
			return
		}
		reply(w, http.StatusNoContent, nil, false, true)
		return
	}
	if r.Method != http.MethodGet {
		deny(http.StatusMethodNotAllowed)
		return
	}
	if health {
		reply(w, http.StatusOK, b.health, false, false)
		return
	}
	select {
	case b.busy <- struct{}{}:
		defer func() { <-b.busy }()
	default:
		deny(http.StatusServiceUnavailable)
		return
	}
	if isFile {
		b.mu.RLock()
		_, ok := b.allowed[file]
		b.mu.RUnlock()
		if !validPath(file) || !ok {
			deny(http.StatusForbidden)
			return
		}
	}
	b.relay(w, r, list, file)
}

func validPath(path string) bool {
	if len(path) == 0 || len(path) > 4096 || !utf8.ValidString(path) || strings.Contains(path, "://") {
		return false
	}
	for _, c := range path {
		if c < 32 || c == 127 {
			return false
		}
	}
	absolute := strings.HasPrefix(path, "/")
	if len(path) >= 3 && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) && path[1] == ':' && (path[2] == '/' || path[2] == '\\') {
		absolute = true
	}
	if !absolute {
		return false
	}
	for _, segment := range strings.FieldsFunc(path, func(c rune) bool { return c == '/' || c == '\\' }) {
		if segment == ".." {
			return false
		}
	}
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".ttf") || strings.HasSuffix(lower, ".otf") ||
		strings.HasSuffix(lower, ".ttc") || strings.HasSuffix(lower, ".otc")
}

func catalog(body []byte) (map[string]struct{}, bool) {
	var root map[string]json.RawMessage
	if json.Unmarshal(body, &root) != nil || root == nil {
		return nil, false
	}
	var files map[string]json.RawMessage
	if json.Unmarshal(root["fontFiles"], &files) != nil || files == nil || len(files) > 50000 {
		return nil, false
	}
	allowed := make(map[string]struct{})
	for path, raw := range files {
		var faces []json.RawMessage
		if json.Unmarshal(raw, &faces) != nil || faces == nil || len(faces) > 256 {
			return nil, false
		}
		for _, rawFace := range faces {
			var face map[string]json.RawMessage
			if json.Unmarshal(rawFace, &face) != nil || face == nil {
				return nil, false
			}
			var family *string
			if json.Unmarshal(face["family"], &family) != nil || family == nil {
				return nil, false
			}
		}
		if validPath(path) {
			allowed[path] = struct{}{}
		}
	}
	return allowed, true
}

func (b *bridge) relay(w http.ResponseWriter, incoming *http.Request, list bool, file string) {
	code := "UPSTREAM_FAILED"
	fail := func() {
		if list {
			b.mu.Lock()
			b.allowed = map[string]struct{}{}
			b.mu.Unlock()
		}
		reply(w, http.StatusBadGateway, []byte(`{"error":"`+code+`"}`), false, false)
	}
	target, limit := upstreamBase+listTarget, listLimit
	if !list {
		target = upstreamBase + filePrefix + strings.ReplaceAll(url.QueryEscape(file), "+", "%20")
		limit = fontLimit
	}
	ctx, cancel := context.WithTimeout(incoming.Context(), 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		fail()
		return
	}
	req.Header.Set("Origin", "https://www.figma.com")
	resp, err := b.client.Do(req)
	if err != nil {
		var timeout net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
			code = "UPSTREAM_TIMEOUT"
		}
		fail()
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fail()
		return
	}
	if resp.ContentLength > limit {
		code = "SIZE_LIMIT"
		fail()
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if int64(len(body)) > limit {
		code = "SIZE_LIMIT"
		fail()
		return
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = "UPSTREAM_TIMEOUT"
		}
		fail()
		return
	}
	if list {
		files, ok := catalog(body)
		if !ok {
			fail()
			return
		}
		b.mu.Lock()
		b.allowed = files
		b.mu.Unlock()
	}
	reply(w, http.StatusOK, body, !list, false)
}
