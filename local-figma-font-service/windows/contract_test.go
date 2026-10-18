package fontservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestContractDelayedUpstreamHasSeparateSendBudget(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	h := newHandler(contractTransport(func(r *http.Request) (*http.Response, error) {
		select {
		case <-time.After(5200 * time.Millisecond):
			return contractResponse(200, contractCatalogBytes(contractFile)), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	}))
	go func() { done <- serve(ctx, []net.Listener{listener}, h) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve shutdown: %v", err)
			}
		case <-time.After(6 * time.Second):
			t.Error("serve failed to stop")
		}
	}()
	request, _ := http.NewRequest("GET", "http://"+listener.Addr().String()+contractCatalog, nil)
	request.Host = "localhost:38474"
	request.Header.Set("Origin", "null")
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: 10 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || !bytes.Equal(body, contractCatalogBytes(contractFile)) {
		t.Fatalf("delayed response %d %q", response.StatusCode, body)
	}
}

const contractCatalog = "/figma/font-files?freetype_minimum_api_version=20&isolate=true&"
const contractFile = `C:\한글 공백\Font.ttf`

type contractTransport func(*http.Request) (*http.Response, error)

func (f contractTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func contractResponse(status int, body []byte) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body))}
}
func contractRequest(h http.Handler, method, target, host, origin string, extra http.Header) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	r.Host = host
	if origin != "<absent>" {
		r.Header["Origin"] = []string{origin}
	}
	for k, v := range extra {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func contractCatalogBytes(paths ...string) []byte {
	files := map[string]any{}
	for _, p := range paths {
		files[p] = []any{map[string]any{"family": "Contract Font"}}
	}
	data, _ := json.Marshal(map[string]any{"fontFiles": files})
	return data
}
func TestContractAccessAndPreflight(t *testing.T) {
	h := newHandler(contractTransport(func(r *http.Request) (*http.Response, error) {
		t.Errorf("unexpected upstream request: %s", r.URL)
		return nil, errors.New("unexpected")
	}))
	cases := []struct {
		name, method, target, host, origin string
		headers                            http.Header
		status                             int
	}{
		{"health", "GET", "/health", "localhost:38474", "null", nil, 200},
		{"absent origin", "GET", "/health", "localhost:38474", "<absent>", nil, 200},
		{"empty origin", "GET", "/health", "localhost:38474", "", nil, 403},
		{"foreign origin", "GET", "/health", "localhost:38474", "https://www.figma.com", nil, 403},
		{"IP host", "GET", "/health", "127.0.0.1:38474", "null", nil, 403},
		{"host case", "GET", "/health", "LOCALHOST:38474", "null", nil, 403},
		{"wrong port", "GET", "/health", "localhost:44950", "null", nil, 403},
		{"unknown route", "GET", "/health/", "localhost:38474", "null", nil, 404},
		{"catalog query exact", "GET", "/figma/font-files?isolate=true&freetype_minimum_api_version=20", "localhost:38474", "null", nil, 404},
		{"POST health", "POST", "/health", "localhost:38474", "null", nil, 405},
		{"preflight", "OPTIONS", contractCatalog, "localhost:38474", "null", http.Header{"Access-Control-Request-Method": {"GET"}}, 204},
		{"preflight denied method", "OPTIONS", contractCatalog, "localhost:38474", "null", http.Header{"Access-Control-Request-Method": {"POST"}}, 403},
		{"preflight headers", "OPTIONS", contractCatalog, "localhost:38474", "null", http.Header{"Access-Control-Request-Method": {"GET"}, "Access-Control-Request-Headers": {"authorization"}}, 403},
		{"preflight missing method", "OPTIONS", contractCatalog, "localhost:38474", "null", nil, 403},
		{"duplicate origin", "GET", "/health", "localhost:38474", "null", http.Header{"Origin": {"null", "null"}}, 400},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := contractRequest(h, c.method, c.target, c.host, c.origin, c.headers)
			if w.Code != c.status {
				t.Fatalf("status %d want %d: %s", w.Code, c.status, w.Body.String())
			}
			for key, want := range map[string]string{"Cache-Control": "no-store", "X-Content-Type-Options": "nosniff", "Access-Control-Allow-Origin": "null", "Vary": "Origin"} {
				if got := w.Header().Get(key); got != want {
					t.Errorf("%s=%q want %q", key, got, want)
				}
			}
			if c.status == 204 {
				for key, want := range map[string]string{"Access-Control-Allow-Methods": "GET", "Access-Control-Allow-Private-Network": "true", "Access-Control-Max-Age": "600"} {
					if got := w.Header().Get(key); got != want {
						t.Errorf("%s=%q", key, got)
					}
				}
			}
		})
	}
}

func TestContractWindowsPathAndAllowlistReset(t *testing.T) {
	var mu sync.Mutex
	catalog := contractCatalogBytes(contractFile, `C:\Windows\Fonts\OTHER.OTF`, `C:\Windows\..\secret.ttf`, `relative.ttf`, `\\server\font.ttf`, `http://evil/font.ttf`)
	calls := 0
	h := newHandler(contractTransport(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if r.URL.Scheme != "http" || r.URL.Host != "localhost:44950" {
			t.Errorf("upstream URL %s", r.URL)
		}
		if r.Header.Get("Origin") != "https://www.figma.com" {
			t.Errorf("upstream Origin %q", r.Header.Get("Origin"))
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("forwarded sensitive client headers")
		}
		if r.URL.RequestURI() == contractCatalog {
			return contractResponse(200, catalog), nil
		}
		if r.URL.Path != "/figma/font-file" || r.URL.Query().Get("file") != contractFile {
			t.Errorf("encoded file %s", r.URL)
		}
		if strings.Contains(r.URL.RawQuery, "+") {
			t.Errorf("upstream path should percent encode spaces: %s", r.URL)
		}
		return contractResponse(200, []byte{0, 1, 0, 0, 7, 9}), nil
	}))
	fileURL := "/figma/font-file?file=" + url.QueryEscape(contractFile)
	if w := contractRequest(h, "GET", fileURL, "localhost:38474", "null", nil); w.Code != 403 {
		t.Fatalf("pre-catalog status %d", w.Code)
	}
	if w := contractRequest(h, "GET", contractCatalog, "localhost:38474", "null", nil); w.Code != 200 {
		t.Fatalf("catalog status %d %s", w.Code, w.Body.String())
	}
	w := contractRequest(h, "GET", fileURL, "localhost:38474", "null", http.Header{"Authorization": {"secret"}, "Cookie": {"secret"}})
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), []byte{0, 1, 0, 0, 7, 9}) || w.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("binary response %d %v", w.Code, w.Body.Bytes())
	}
	for _, p := range []string{`C:\Windows\..\secret.ttf`, `relative.ttf`, `\\server\font.ttf`, `http://evil/font.ttf`, `C:\not-advertised.ttf`, `C:\bad` + string(rune(1)) + `.ttf`} {
		if w := contractRequest(h, "GET", "/figma/font-file?file="+url.QueryEscape(p), "localhost:38474", "null", nil); w.Code != 403 {
			t.Errorf("unsafe path %q status %d", p, w.Code)
		}
	}
	if w := contractRequest(h, "GET", fileURL+"&extra=1", "localhost:38474", "null", nil); w.Code != 404 {
		t.Errorf("extra query status %d", w.Code)
	}
	mu.Lock()
	catalog = []byte(`{"fontFiles":{"C:\\secret.ttf":[{"family":1}]}}`)
	mu.Unlock()
	if w := contractRequest(h, "GET", contractCatalog, "localhost:38474", "null", nil); w.Code != 502 {
		t.Errorf("bad catalog status %d", w.Code)
	}
	if w := contractRequest(h, "GET", fileURL, "localhost:38474", "null", nil); w.Code != 403 {
		t.Errorf("failed refresh did not reset allowlist: %d", w.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 3 {
		t.Errorf("unsafe request reached upstream: calls=%d want 3", calls)
	}
}

func TestContractUpstreamFailureIsSanitized(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		err    error
	}{
		{"redirect", 302, nil}, {"upstream status", 500, nil}, {"transport failure", 0, errors.New(`C:\Users\secret\font.ttf authorization=secret`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHandler(contractTransport(func(*http.Request) (*http.Response, error) {
				if test.err != nil {
					return nil, test.err
				}
				r := contractResponse(test.status, []byte("private diagnostic"))
				r.Header.Set("Location", "http://evil.invalid")
				return r, nil
			}))
			w := contractRequest(h, "GET", contractCatalog, "localhost:38474", "null", nil)
			if w.Code != 502 || w.Body.String() != `{"error":"UPSTREAM_FAILED"}` {
				t.Fatalf("response %d %q", w.Code, w.Body.String())
			}
		})
	}
}

func TestContractConcurrentRefreshAndRead(t *testing.T) {
	h := newHandler(contractTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/figma/font-files" {
			return contractResponse(200, contractCatalogBytes(contractFile)), nil
		}
		return contractResponse(200, []byte("font")), nil
	}))
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := contractCatalog
			if i%2 != 0 {
				target = "/figma/font-file?file=" + url.QueryEscape(contractFile)
			}
			w := contractRequest(h, "GET", target, "localhost:38474", "null", nil)
			if w.Code != 200 && w.Code != 403 && w.Code != 503 {
				t.Errorf("concurrent status %d", w.Code)
			}
		}(i)
	}
	wg.Wait()
}

func TestContractRawHeaderGuard(t *testing.T) {
	good := "GET /health HTTP/1.1\r\nHost: localhost:38474\r\nOrigin: null"
	for _, header := range []string{good, good + "\r\nContent-Length: 0", "OPTIONS " + contractCatalog + " HTTP/1.1\r\nHost: localhost:38474\r\nAccess-Control-Request-Method: GET"} {
		if !validHeader([]byte(header)) {
			t.Errorf("valid header rejected: %q", header)
		}
	}
	for _, header := range []string{
		good + "\r\nContent-Length: 0\r\ncontent-length: 0", good + "\r\nTransfer-Encoding: chunked", good + "\r\nContent-Length: 1",
		good + "\r\nOrigin: null", good + "\r\nHost: localhost:38474", good + "\r\n X-Folded: value", good + "\r\nNul: \x00",
		"GET http://localhost:38474/health HTTP/1.1\r\nHost: localhost:38474", "GET /health HTTP/1.0\r\nHost: localhost:38474",
		"GET /health#fragment HTTP/1.1\r\nHost: localhost:38474", good + "\r\nBrokenHeader",
	} {
		if validHeader([]byte(header)) {
			t.Errorf("unsafe raw header accepted: %q", header)
		}
	}
}

// Explicit opt-in: this talks to the real Figma font agent on the host, without
// starting Run or touching the existing native macOS relay at port 38474.
func TestContractRealFigmaUpstream(t *testing.T) {
	if os.Getenv("FIGMAPDF_REAL_FIGMA_TEST") != "1" {
		t.Skip("requires live Figma font agent on localhost:44950")
	}
	h := NewHandler()
	w := contractRequest(h, "GET", contractCatalog, "localhost:38474", "null", nil)
	if w.Code != 200 {
		t.Fatalf("live catalog %d %s", w.Code, w.Body.String())
	}
	var catalog struct {
		FontFiles map[string]json.RawMessage `json:"fontFiles"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &catalog); err != nil || len(catalog.FontFiles) == 0 {
		t.Fatalf("live catalog invalid/empty: %v", err)
	}
	var path string
	for candidate := range catalog.FontFiles {
		if strings.HasSuffix(strings.ToLower(candidate), ".ttf") || strings.HasSuffix(strings.ToLower(candidate), ".otf") {
			path = candidate
			break
		}
	}
	if path == "" {
		t.Fatal("no TTF/OTF catalog entry")
	}
	w = contractRequest(h, "GET", "/figma/font-file?file="+url.QueryEscape(path), "localhost:38474", "null", nil)
	if w.Code != 200 || w.Body.Len() == 0 {
		t.Fatalf("live binary %d, bytes=%d", w.Code, w.Body.Len())
	}
	req, _ := http.NewRequest("GET", "http://localhost:44950/figma/font-file?file="+url.QueryEscape(path), nil)
	req.Header.Set("Origin", "https://www.figma.com")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 15 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 20000001))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || !bytes.Equal(raw, w.Body.Bytes()) {
		t.Fatal("relay bytes differ from original Figma upstream")
	}
	t.Logf("real Figma upstream: catalog entries=%d; binary bytes=%d; byte equality=true; no Windows listener/lifecycle claim", len(catalog.FontFiles), len(raw))
}

// The existing launchd relay remains installed and owns 38474. This compares its
// externally visible contract to the Go handler without rebinding that port.
func TestContractExistingMacParity(t *testing.T) {
	if os.Getenv("FIGMAPDF_REAL_FIGMA_TEST") != "1" {
		t.Skip("requires existing native macOS relay and live Figma")
	}
	h := NewHandler()
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: 15 * time.Second}
	for _, c := range []struct{ method, target, host, origin, preflight string }{
		{"GET", "/health", "localhost:38474", "null", ""},
		{"GET", "/health", "localhost:38474", "<absent>", ""},
		{"GET", "/health", "127.0.0.1:38474", "null", ""},
		{"GET", "/health", "localhost:38474", "https://www.figma.com", ""},
		{"GET", "/unknown", "localhost:38474", "null", ""},
		{"POST", "/health", "localhost:38474", "null", ""},
		{"OPTIONS", contractCatalog, "localhost:38474", "null", "GET"},
		{"OPTIONS", contractCatalog, "localhost:38474", "null", "POST"},
		{"GET", "/figma/font-file?file=" + url.QueryEscape(contractFile), "localhost:38474", "null", ""},
		{"GET", contractCatalog, "localhost:38474", "null", ""},
	} {
		t.Run(c.method+" "+c.target+" "+c.host+" "+c.origin+" "+c.preflight, func(t *testing.T) {
			extra := http.Header{}
			if c.preflight != "" {
				extra.Set("Access-Control-Request-Method", c.preflight)
			}
			local := contractRequest(h, c.method, c.target, c.host, c.origin, extra)
			req, _ := http.NewRequest(c.method, "http://127.0.0.1:38474"+c.target, nil)
			req.Host = c.host
			if c.origin != "<absent>" {
				req.Header.Set("Origin", c.origin)
			}
			req.Header.Set("Access-Control-Request-Method", c.preflight)
			native, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer native.Body.Close()
			body, err := io.ReadAll(io.LimitReader(native.Body, 8000001))
			if err != nil {
				t.Fatal(err)
			}
			if local.Code != native.StatusCode {
				t.Errorf("Go=%d macOS=%d", local.Code, native.StatusCode)
			}
			for _, key := range []string{"Content-Type", "Cache-Control", "X-Content-Type-Options", "Access-Control-Allow-Origin", "Vary", "Access-Control-Allow-Methods", "Access-Control-Max-Age", "Access-Control-Allow-Private-Network"} {
				if local.Header().Get(key) != native.Header.Get(key) {
					t.Errorf("%s Go=%q macOS=%q", key, local.Header().Get(key), native.Header.Get(key))
				}
			}
			if c.target != "/health" && !bytes.Equal(local.Body.Bytes(), body) {
				t.Errorf("Go and native response bytes differ")
			}
		})
	}
}
