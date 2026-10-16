// Package redact keeps request URLs out of log lines and error text.
//
// Every error http.Client.Do returns is a *url.Error whose text starts with
// the method and the full request URL ("Get \"https://host/path?q\": ..."),
// and a request URL carries the caller's arguments in its path and query.
// Anything that logs an outbound request's error passes it through Error
// first; anything that logs a URL it was given logs Host(u) instead.
//
// What it cannot do: Error finds a URL through the *url.Error in an error's
// chain. Once a library has flattened that error into a new one with %v (not
// %w), the chain is gone and the URL is plain text Error cannot recognise, so
// it passes through unchanged. A caller wrapping a library that flattens
// errors must not rely on Error; it has to log a fixed text instead.
package redact

import (
	"net/url"
	"strings"
)

// Error returns err with the method and URL of every *url.Error in its chain
// removed from its text: each one's text is replaced by its wrapped error's
// text, and any copy of its URL that survives (a wrapper that quoted the text
// differently) is replaced by its host. The result still unwraps to err, so
// errors.Is and errors.As see exactly what they saw before and a classifier
// that reads types gives the same answer. An error with no *url.Error in its
// chain is returned unchanged; nil stays nil.
func Error(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	found := false
	walk(err, func(e error) {
		ue, ok := e.(*url.Error)
		if !ok {
			return
		}
		found = true
		inner := ""
		if ue.Err != nil {
			inner = ue.Err.Error()
		}
		msg = strings.ReplaceAll(msg, ue.Error(), inner)
		if ue.URL != "" {
			msg = strings.ReplaceAll(msg, ue.URL, Host(ue.URL))
		}
	})
	if !found {
		return err
	}
	return &scrubbed{err: err, msg: msg}
}

// scrubbed is an error whose text was rewritten by Error; it unwraps to the
// original.
type scrubbed struct {
	err error
	msg string
}

func (s *scrubbed) Error() string { return s.msg }
func (s *scrubbed) Unwrap() error { return s.err }

// Host returns raw reduced to "scheme://host[:port]": no userinfo, path,
// query or fragment. A value that does not parse as an absolute URL gives
// "[url]", never the input.
func Host(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "[url]"
	}
	return u.Scheme + "://" + u.Host
}

// walk calls f on err and every error it wraps, through both Unwrap() error
// and Unwrap() []error.
func walk(err error, f func(error)) {
	for err != nil {
		f(err)
		switch u := err.(type) {
		case interface{ Unwrap() error }:
			err = u.Unwrap()
		case interface{ Unwrap() []error }:
			for _, e := range u.Unwrap() {
				walk(e, f)
			}
			return
		default:
			return
		}
	}
}
