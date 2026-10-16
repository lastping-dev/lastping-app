package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/lastping-dev/lastping-app/internal/redact"
)

// This file is the only place in the package that sends a request or reads a
// response body (TestResponseHandling_OnlyInResponseFile enforces it). Tool
// output is shown to the user and the assistant, and every error the
// transport hands back while building, sending or reading names things that
// do not belong there: the request URL (with the caller's arguments in its
// path and query) and the addresses the connection used ("read tcp
// 192.0.2.10:41234->192.0.2.20:443: connection reset by peer"). So each
// failure here becomes one of the fixed sentences below and its detail goes
// to the log (stderr), with the request URL removed.

// errAPIUnreachable is the error for a request that got no HTTP response.
var errAPIUnreachable = errors.New("the LastPing API could not be reached; try again in a minute")

// errAPITimeout is the error for a request, or the read of its response,
// that ran out of time. "Could not be reached" would be wrong: the API may
// well have received it.
var errAPITimeout = errors.New("the LastPing API did not respond in time; try again")

// errResponseInterrupted is the error for a response whose body broke off
// before it was complete (a reset or a closed connection mid-body).
var errResponseInterrupted = errors.New("the LastPing API response was interrupted; try again in a minute")

// errUnexpectedResponse is the error for a complete response body that does
// not decode into what the tool expects: a contract mismatch, never a
// transport failure. The decoder's own text is not shown.
var errUnexpectedResponse = errors.New("unexpected response from the LastPing API")

// errBuildRequest is the error for a request that could not be built. The
// only way that happens is a URL that does not parse, and the parser's text
// quotes the whole URL, so the sentence names the two things that make one:
// the configured base URL and the arguments placed in its path.
var errBuildRequest = errors.New("could not build the LastPing API request; check that LASTPING_BASE_URL is a valid URL and that no argument contains a control character")

// buildFailed logs why http.NewRequest failed, without the URL, and returns
// errBuildRequest.
func buildFailed(err error) error {
	log.Printf("mcptools: building a LastPing API request failed: %v", redact.Error(err))
	return errBuildRequest
}

// isTimeout reports whether err is a deadline running out, judged by type
// and sentinel, never by the error's text (which includes the URL).
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// do sends req with c.HTTP. A transport failure is logged without the
// request URL (redact.Error drops it; the path and query are not logged
// either) and returned as errAPITimeout or errAPIUnreachable, never as its
// own text.
func (c *APIClient) do(req *http.Request) (*http.Response, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		log.Printf("mcptools: %s request to the LastPing API failed: %v", req.Method, redact.Error(err))
		if isTimeout(err) {
			return nil, errAPITimeout
		}
		return nil, errAPIUnreachable
	}
	return resp, nil
}

// readBody reads resp's body, at most limit bytes when limit > 0. A read
// failure is logged and returned as errAPITimeout or errResponseInterrupted.
func readBody(resp *http.Response, limit int64) ([]byte, error) {
	var r io.Reader = resp.Body
	if limit > 0 {
		r = io.LimitReader(resp.Body, limit)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		log.Printf("mcptools: reading a LastPing API response (HTTP %d) failed: %v", resp.StatusCode, redact.Error(err))
		if isTimeout(err) {
			return nil, errAPITimeout
		}
		return nil, errResponseInterrupted
	}
	return b, nil
}

// decodeBody reads resp's whole body and decodes it into v. A read failure
// returns readBody's fixed error; a complete body that does not decode
// returns errUnexpectedResponse (the decoder's text goes to the log only).
func decodeBody(resp *http.Response, v any) error {
	b, err := readBody(resp, 0)
	if err != nil {
		return err
	}
	return unmarshalBody(b, v)
}

// unmarshalBody decodes an already-read response body into v, returning
// errUnexpectedResponse on failure.
func unmarshalBody(b []byte, v any) error {
	if err := json.Unmarshal(b, v); err != nil {
		log.Printf("mcptools: a LastPing API response did not decode: %v", err)
		return errUnexpectedResponse
	}
	return nil
}
