package fontservice

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func upstream(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}

func TestServeCancellationCancelsUpstream(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	handler := newHandler(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serve(ctx, []net.Listener{listener}, handler) }()
	request, _ := http.NewRequest("GET", "http://"+listener.Addr().String()+listTarget, nil)
	request.Host = "localhost:38474"
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
		if err == nil {
			response.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream not reached")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not cancel upstream")
	}
	<-clientDone
	if conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second); err == nil {
		conn.Close()
		t.Fatal("listener remains open")
	}
}

func TestIPv6UnavailableExcludesPortConflict(t *testing.T) {
	if ipv6Unavailable(&net.OpError{Err: syscall.EADDRINUSE}) {
		t.Fatal("port conflict silently ignored")
	}
	if !ipv6Unavailable(&net.OpError{Err: syscall.EAFNOSUPPORT}) {
		t.Fatal("unsupported IPv6 was not optional")
	}
}
func call(h http.Handler, target string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", target, nil)
	r.Host = "localhost:38474"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCatalogLifecycle(t *testing.T) {
	path := `C:\Fonts\Family.ttf`
	listing := `{"fontFiles":{"C:\\Fonts\\Family.ttf":[{"family":"Family"}]}}`
	fail := false
	b := newHandler(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Origin") != "https://www.figma.com" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected upstream headers")
		}
		if r.URL.Scheme != "http" || r.URL.Host != "localhost:44950" {
			t.Error("upstream endpoint changed")
		}
		if r.URL.Path == "/figma/font-files" {
			if fail {
				return upstream(`{"fontFiles":null}`), nil
			}
			return upstream(listing), nil
		}
		if r.URL.Query().Get("file") != path {
			t.Errorf("file encoding: %s", r.URL.RawQuery)
		}
		return upstream("font"), nil
	}))
	target := filePrefix + url.QueryEscape(path)
	if w := call(b, target); w.Code != 403 {
		t.Fatalf("fresh allowlist: %d", w.Code)
	}
	if w := call(b, listTarget); w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if w := call(b, target); w.Code != 200 || w.Body.String() != "font" {
		t.Fatalf("font: %d", w.Code)
	}
	fail = true
	if w := call(b, listTarget); w.Code != 502 {
		t.Fatalf("invalid list: %d", w.Code)
	}
	if w := call(b, target); w.Code != 403 {
		t.Fatalf("failed list must clear allowlist: %d", w.Code)
	}
}

func TestBusyDoesNotBlockHealth(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	b := newHandler(roundTripFunc(func(*http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return upstream(`{"fontFiles":{}}`), nil
	}))
	go func() { defer close(done); call(b, listTarget) }()
	<-entered
	if w := call(b, listTarget); w.Code != 503 {
		t.Errorf("busy: %d", w.Code)
	}
	if w := call(b, "/health"); w.Code != 200 {
		t.Errorf("health blocked: %d", w.Code)
	}
	close(release)
	<-done
}

func TestCatalogAndPathValidation(t *testing.T) {
	for _, path := range []string{`C:\Fonts\one.ttf`, `d:/Fonts/one.OTF`, `/fonts/one.ttf`, `C:\Fonts\static.ttc`, `/fonts/static.OTC`} {
		if !validPath(path) {
			t.Errorf("valid path rejected: %q", path)
		}
	}
	for _, path := range []string{`C:one.ttf`, `\\server\font.ttf`, `fonts/a.ttf`, `/fonts/../a.ttf`, `C:\Fonts\..\a.ttf`, `/a.collection`, `http://x/a.ttf`, "/a\x00.ttf"} {
		if validPath(path) {
			t.Errorf("unsafe path accepted: %q", path)
		}
	}
	for _, body := range []string{`null`, `[]`, `{}`, `{"fontFiles":null}`, `{"fontFiles":[]}`, `{"fontFiles":{"/a.ttf":null}}`, `{"fontFiles":{"/a.ttf":[null]}}`, `{"fontFiles":{"/a.ttf":[{"family":null}]}}`, `{"fontFiles":{"/a.ttf":[{"family":12}]}}`} {
		if _, ok := catalog([]byte(body)); ok {
			t.Errorf("invalid catalog accepted: %s", body)
		}
	}
	files, ok := catalog([]byte(`{"fontFiles":{"/safe.ttf":[{"family":""}],"/static.ttc":[{"family":"Collection"}]}}`))
	if !ok || len(files) != 2 {
		t.Fatal("supported file extensions must enter allowlist")
	}
	if _, exists := files["/static.ttc"]; !exists {
		t.Fatal("collection catalog path was not allowlisted")
	}
}

func TestCollectionCatalogAllowlistAndResponseLimit(t *testing.T) {
	const collectionPath = `C:\Windows\Fonts\Static.ttc`
	list := `{"fontFiles":{"C:\\Windows\\Fonts\\Static.ttc":[{"family":"Static Collection"}]}}`
	requests := 0
	b := newHandler(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/figma/font-files" {
			return upstream(list), nil
		}
		requests++
		if r.URL.Query().Get("file") != collectionPath {
			t.Fatalf("unexpected file request: %s", r.URL)
		}
		return upstream("static collection bytes"), nil
	}))
	if w := call(b, listTarget); w.Code != http.StatusOK {
		t.Fatalf("catalog: %d %s", w.Code, w.Body.String())
	}
	known := filePrefix + url.QueryEscape(collectionPath)
	if w := call(b, known); w.Code != http.StatusOK || w.Body.String() != "static collection bytes" {
		t.Fatalf("allowlisted collection: %d %s", w.Code, w.Body.String())
	}
	if w := call(b, filePrefix+url.QueryEscape(`C:\Windows\Fonts\Unknown.ttc`)); w.Code != http.StatusForbidden {
		t.Fatalf("unknown collection path: %d", w.Code)
	}
	if requests != 1 {
		t.Fatalf("unexpected upstream file requests: %d", requests)
	}
	if fontLimit != 20_000_000 {
		t.Fatalf("collection response cap changed: %d", fontLimit)
	}
}

func TestUpstreamFailureBounds(t *testing.T) {
	tests := []struct {
		name string
		trip roundTripFunc
		code string
	}{
		{"timeout", func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }, "UPSTREAM_TIMEOUT"},
		{"redirect", func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"http://elsewhere.invalid/"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}, "UPSTREAM_FAILED"},
		{"declared size", func(*http.Request) (*http.Response, error) {
			r := upstream("")
			r.ContentLength = listLimit + 1
			return r, nil
		}, "SIZE_LIMIT"},
		{"stream size", func(*http.Request) (*http.Response, error) {
			r := upstream(strings.Repeat("x", int(listLimit)+1))
			r.ContentLength = -1
			return r, nil
		}, "SIZE_LIMIT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := newHandler(test.trip)
			b.allowed["/old.ttf"] = struct{}{}
			w := call(b, listTarget)
			if w.Code != 502 || !strings.Contains(w.Body.String(), test.code) {
				t.Fatalf("response %d %s", w.Code, w.Body.String())
			}
			if len(b.allowed) != 0 {
				t.Fatal("failure did not clear allowlist")
			}
		})
	}
}

func TestBinaryBodyLimit(t *testing.T) {
	b := newHandler(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, ContentLength: -1,
			Body: io.NopCloser(io.LimitReader(zeroReader{}, fontLimit+1))}, nil
	}))
	b.allowed["/font.ttc"] = struct{}{}
	w := call(b, filePrefix+url.QueryEscape("/font.ttc"))
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "SIZE_LIMIT") {
		t.Fatalf("binary limit: %d %s", w.Code, w.Body.String())
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
