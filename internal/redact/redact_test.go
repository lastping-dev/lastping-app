package redact_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lastping-dev/lastping-app/internal/redact"
)

const secretURL = "https://user:pw@hooks.example.com/services/T0/B0/SECRET123?token=SECRET456#frag"

func TestError_DropsURLKeepsCause(t *testing.T) {
	cause := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	cases := map[string]error{
		"bare":             &url.Error{Op: "Post", URL: secretURL, Err: cause},
		"wrapped":          fmt.Errorf("slack: do request: %w", &url.Error{Op: "Post", URL: secretURL, Err: cause}),
		"joined":           errors.Join(errors.New("first"), &url.Error{Op: "Get", URL: secretURL, Err: cause}),
		"quoted by a %q":   fmt.Errorf("outer %q: %w", (&url.Error{Op: "Get", URL: secretURL, Err: cause}).Error(), &url.Error{Op: "Get", URL: secretURL, Err: cause}),
		"url only in a %v": fmt.Errorf("sdk: request send failed, %v (%w)", "Get "+secretURL, &url.Error{Op: "Get", URL: secretURL, Err: cause}),
	}
	for name, err := range cases {
		t.Run(name, func(t *testing.T) {
			got := redact.Error(err)
			for _, leak := range []string{"SECRET123", "SECRET456", "user:pw", "/services/", "frag"} {
				if strings.Contains(got.Error(), leak) {
					t.Fatalf("redacted text %q carries %q", got.Error(), leak)
				}
			}
			if !strings.Contains(got.Error(), "connection reset by peer") {
				t.Fatalf("redacted text %q lost the cause", got.Error())
			}
			if !errors.Is(got, syscall.ECONNRESET) {
				t.Fatal("errors.Is lost the cause")
			}
			var ue *url.Error
			if !errors.As(got, &ue) {
				t.Fatal("errors.As no longer reaches the *url.Error")
			}
		})
	}
}

// A real client.Do failure, with a timeout: the text keeps "timeout", which
// the prober's classification and the log both read.
func TestError_RealClientTimeout(t *testing.T) {
	c := &http.Client{Timeout: 50 * time.Millisecond, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}}
	req, _ := http.NewRequest(http.MethodGet, "http://probe.example/health?token=SECRET123", nil)
	_, err := c.Do(req)
	if err == nil || !strings.Contains(err.Error(), "SECRET123") {
		t.Fatalf("precondition: client error %v should carry the URL", err)
	}
	got := redact.Error(err)
	if strings.Contains(got.Error(), "SECRET123") || strings.Contains(got.Error(), "probe.example") {
		t.Fatalf("redacted %q", got.Error())
	}
	if !strings.Contains(got.Error(), "Client.Timeout exceeded") {
		t.Fatalf("lost the timeout: %q", got.Error())
	}
	var ne net.Error
	if !errors.As(got, &ne) || !ne.Timeout() {
		t.Fatal("Timeout() no longer reachable")
	}
}

func TestError_NoURLErrorIsUnchanged(t *testing.T) {
	if redact.Error(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	plain := errors.New("status 500")
	if redact.Error(plain) != plain {
		t.Fatal("an error without a *url.Error must be returned as is")
	}
}

func TestHost(t *testing.T) {
	cases := map[string]string{
		secretURL:                        "https://hooks.example.com",
		"http://10.0.0.1:8080/x?y=z":     "http://10.0.0.1:8080",
		"not a url":                      "[url]",
		"":                               "[url]",
		"/relative/path?token=SECRET123": "[url]",
	}
	for in, want := range cases {
		if got := redact.Host(in); got != want {
			t.Errorf("Host(%q) = %q, want %q", in, got, want)
		}
	}
}
