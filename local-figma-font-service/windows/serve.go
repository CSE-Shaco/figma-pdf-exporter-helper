package fontservice

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

const headerLimit = 8192

// Run serves only loopback endpoints until cancellation. IPv4 is mandatory;
// IPv6 is used whenever the local machine makes it available.
func Run(ctx context.Context) error {
	v4, err := net.Listen("tcp4", "127.0.0.1:38474")
	if err != nil {
		return err
	}
	listeners := []net.Listener{v4}
	if v6, err := net.Listen("tcp6", "[::1]:38474"); err == nil {
		listeners = append(listeners, v6)
	} else if !ipv6Unavailable(err) {
		_ = v4.Close()
		return err
	}
	return serve(ctx, listeners, NewHandler())
}

func serve(ctx context.Context, listeners []net.Listener, handler http.Handler) error {
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: headerLimit,
		ErrorLog: log.New(io.Discard, "", 0), BaseContext: func(net.Listener) context.Context { return ctx }}
	server.SetKeepAlivesEnabled(false)
	slots := make(chan struct{}, 64)
	results := make(chan error, len(listeners))
	for _, listener := range listeners {
		go func(l net.Listener) { results <- server.Serve(&guardListener{Listener: l, slots: slots}) }(listener)
	}
	var result error
	select {
	case <-ctx.Done():
	case result = <-results:
		if errors.Is(result, http.ErrServerClosed) {
			result = nil
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		_ = server.Close()
	}
	return result
}

func ipv6Unavailable(err error) bool {
	if errors.Is(err, syscall.EAFNOSUPPORT) || errors.Is(err, syscall.EPROTONOSUPPORT) || errors.Is(err, syscall.EADDRNOTAVAIL) {
		return true
	}
	// Winsock errors differ from Go's portable syscall errno constants.
	if runtime.GOOS == "windows" {
		var errno syscall.Errno
		if errors.As(err, &errno) {
			return errno == 10047 || errno == 10043 || errno == 10049
		}
	}
	return false
}

type guardListener struct {
	net.Listener
	slots chan struct{}
}

func (l *guardListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		select {
		case l.slots <- struct{}{}:
			return &guardConn{Conn: conn, slots: l.slots}, nil
		default:
			_ = conn.Close()
		}
	}
}

// Guard before net/http's parser normalizes duplicate Content-Length headers.
// It also enforces the exact header budget rather than net/http's parser slop.
type guardConn struct {
	net.Conn
	slots   chan struct{}
	once    sync.Once
	checked bool
	buffer  []byte
}

func (c *guardConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { <-c.slots })
	return err
}
func (c *guardConn) Read(p []byte) (int, error) {
	if !c.checked {
		c.checked = true
		_ = c.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		buffer := make([]byte, 0, headerLimit)
		for {
			chunk := make([]byte, min(1024, headerLimit-len(buffer)))
			n, err := c.Conn.Read(chunk)
			buffer = append(buffer, chunk[:n]...)
			if end := bytes.Index(buffer, []byte("\r\n\r\n")); end >= 0 {
				if !validHeader(buffer[:end]) || len(buffer) > end+4 {
					c.reject("400 Bad Request")
					return 0, io.EOF
				}
				c.buffer = buffer
				break
			}
			if len(buffer) >= headerLimit {
				c.reject("431 Request Header Fields Too Large")
				return 0, io.EOF
			}
			if err != nil {
				if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
					c.reject("408 Request Timeout")
				}
				return 0, err
			}
		}
	}
	if len(c.buffer) > 0 {
		n := copy(p, c.buffer)
		c.buffer = c.buffer[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}
func (c *guardConn) reject(status string) {
	_ = c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(c.Conn, "HTTP/1.1 "+status+"\r\nContent-Length: 0\r\nConnection: close\r\nCache-Control: no-store\r\nAccess-Control-Allow-Origin: null\r\nVary: Origin\r\nX-Content-Type-Options: nosniff\r\n\r\n")
	_ = c.Close()
}
func validHeader(header []byte) bool {
	if bytes.IndexByte(header, 0) >= 0 {
		return false
	}
	lines := strings.Split(string(header), "\r\n")
	request := strings.Split(lines[0], " ")
	if len(request) != 3 || request[2] != "HTTP/1.1" || !strings.HasPrefix(request[1], "/") {
		return false
	}
	target, err := url.ParseRequestURI(request[1])
	if err != nil || target.IsAbs() || target.Host != "" || strings.Contains(request[1], "#") {
		return false
	}
	seen := make(map[string]bool)
	for _, line := range lines[1:] {
		name, value, ok := strings.Cut(line, ":")
		if !ok || name == "" || strings.TrimSpace(name) != name {
			return false
		}
		name = strings.ToLower(name)
		value = strings.Trim(value, " \t")
		switch name {
		case "transfer-encoding":
			return false
		case "content-length":
			if value != "0" {
				return false
			}
			fallthrough
		case "host", "origin", "access-control-request-method", "access-control-request-headers":
			if seen[name] {
				return false
			}
			seen[name] = true
		}
	}
	return true
}
