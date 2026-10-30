package provider

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestNewStreamingHTTPClient_Transport guards the config of the shared
// streaming client: it must bound the wait for response headers (so a silent
// upstream cannot hang client.Do forever) while NOT setting an overall
// Client.Timeout (which would kill legitimate long generations).
func TestNewStreamingHTTPClient_Transport(t *testing.T) {
	c := newStreamingHTTPClient()
	if c.Timeout != 0 {
		t.Errorf("Client.Timeout = %v, want 0 (must not cap long streams)", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", c.Transport)
	}
	if tr.ResponseHeaderTimeout != defaultResponseHeaderTimeout {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", tr.ResponseHeaderTimeout, defaultResponseHeaderTimeout)
	}
}

// TestOpenAIStream_SilentUpstreamDoesNotHang reproduces the production hang:
// the upstream accepts the TCP connection but never sends HTTP response
// headers. Before the fix (bare &http.Client{} with no transport timeouts and
// a context without deadline) this blocked forever. Now ResponseHeaderTimeout
// must abort the request within a bounded time.
func TestOpenAIStream_SilentUpstreamDoesNotHang(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Accept connections and hold them open without ever writing a response.
	// Closing the listener at test end unblocks the Accept loop.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				buf := make([]byte, 1024)
				for {
					if _, err := c.Read(buf); err != nil {
						_ = c.Close()
						return
					}
				}
			}(conn)
		}
	}()

	p, err := NewOpenAIProvider("test", ProviderConfig{
		Type:    "openai",
		APIKey:  "k",
		Model:   "m",
		BaseURL: "http://" + ln.Addr().String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Shrink the header timeout for the test so it stays fast, and disable
	// provider-level retries so we assert the pure ResponseHeaderTimeout
	// mechanism (with the default 3 retries the total is still bounded, just
	// a few seconds longer — see TestOpenAIStream_SilentUpstreamBoundedWithRetries).
	if tr, ok := p.client.Transport.(*http.Transport); ok {
		tr.ResponseHeaderTimeout = 300 * time.Millisecond
	}
	p.SetRetryPolicy(NoRetryPolicy())

	start := time.Now()
	_, err = p.StreamWithCtx(context.Background(), []Message{UserMsg("hi")}, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from silent upstream, got nil")
	}
	// Must return promptly (bounded by ResponseHeaderTimeout), not hang.
	if elapsed > 3*time.Second {
		t.Fatalf("request hung for %v; ResponseHeaderTimeout not applied", elapsed)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Logf("error (informational, still bounded): %v", err)
	}
}
