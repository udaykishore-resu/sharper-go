package retry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// script is a fake transport: each call returns the next step.
// It counts bodies handed out and bodies closed.
type script struct {
	steps          []step
	calls          int
	opened, closed atomic.Int32
}

type step struct {
	status int
	header http.Header
	err    error
}

type body struct {
	io.Reader
	s *script
}

func (b body) Close() error { b.s.closed.Add(1); return nil }

func (s *script) RoundTrip(*http.Request) (*http.Response, error) {
	st := s.steps[min(s.calls, len(s.steps)-1)]
	s.calls++
	if st.err != nil {
		return nil, st.err
	}
	s.opened.Add(1)
	return &http.Response{StatusCode: st.status,
		Header: cmpHeader(st.header),
		Body:   body{strings.NewReader("call " + string(rune('0'+s.calls))), s}}, nil
}

func cmpHeader(h http.Header) http.Header {
	if h == nil {
		return http.Header{}
	}
	return h
}

func mk() (*http.Request, error) { return http.NewRequest("GET", "http://dep/", nil) }

var policy = Policy{Attempts: 3, Base: 10 * time.Millisecond, Max: 2 * time.Second}

func TestPaths(t *testing.T) {
	for _, c := range []struct {
		name      string
		steps     []step
		budget    float64
		wantCalls int
		wantCode  int // 0: want an error
		wantBody  string
	}{
		{"503 then 200", []step{{status: 503}, {status: 200}}, 10, 2, 200, "call 2"},
		{"network error then 200", []step{{err: io.ErrUnexpectedEOF}, {status: 200}}, 10, 2, 200, "call 2"},
		{"422 is returned at once", []step{{status: 422}}, 10, 1, 422, "call 1"},
		{"attempts exhausted: last 503 intact", []step{{status: 503}}, 10, 3, 503, "call 3"},
		{"empty budget: first 503 intact", []step{{status: 503}}, 0, 1, 503, "call 1"},
		{"network errors exhausted", []step{{err: io.EOF}}, 10, 3, 0, ""},
		{"Retry-After past Max: give up", []step{{status: 429,
			header: http.Header{"Retry-After": {"3"}}}}, 10, 1, 429, "call 1"},
	} {
		synctest.Test(t, func(t *testing.T) {
			s := &script{steps: c.steps}
			cl := &http.Client{Transport: s}
			resp, err := Do(t.Context(), cl, policy, NewBudget(c.budget, 0.1), mk)
			if s.calls != c.wantCalls {
				t.Errorf("%s: %d calls, want %d", c.name, s.calls, c.wantCalls)
			}
			if c.wantCode == 0 {
				if err == nil || resp != nil {
					t.Errorf("%s: resp=%v err=%v", c.name, resp, err)
				}
			} else {
				if err != nil || resp.StatusCode != c.wantCode {
					t.Fatalf("%s: resp=%v err=%v", c.name, resp, err)
				}
				b, _ := io.ReadAll(resp.Body) // intact, unread
				if string(b) != c.wantBody {
					t.Errorf("%s: body %q, want %q", c.name, b, c.wantBody)
				}
				resp.Body.Close()
			}
			if o, cl := s.opened.Load(), s.closed.Load(); o != cl {
				t.Errorf("%s: %d bodies opened, %d closed", c.name, o, cl)
			}
		})
	}
}

// Both Retry-After forms are honoured, and the wait is the larger
// of the header and the backoff.
func TestRetryAfterForms(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, date := range []bool{false, true} {
			v := "1"
			if date {
				v = time.Now().Add(1500 * time.Millisecond).UTC().Format(http.TimeFormat)
			}
			s := &script{steps: []step{{status: 503,
				header: http.Header{"Retry-After": {v}}}, {status: 200}}}
			start := time.Now()
			resp, err := Do(t.Context(), &http.Client{Transport: s}, policy,
				NewBudget(10, 0.1), mk)
			if err != nil || resp.StatusCode != 200 {
				t.Fatalf("%q: resp=%v err=%v", v, resp, err)
			}
			resp.Body.Close()
			if waited := time.Since(start); waited < time.Second {
				t.Errorf("Retry-After %q: waited only %v", v, waited)
			}
		}
	})
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	for v, want := range map[string]time.Duration{
		"7":                             7 * time.Second,
		"Sun, 11 Oct 2026 12:00:30 GMT": 30 * time.Second,
		"Sun, 11 Oct 2026 11:00:00 GMT": 0, // in the past
		"soon":                          0,
		"-5":                            0,
	} {
		resp := &http.Response{Header: http.Header{"Retry-After": {v}}}
		if got := retryAfter(resp, now); got != want {
			t.Errorf("retryAfter(%q) = %v, want %v", v, got, want)
		}
	}
}

// A wait that would pass the caller's deadline gives up at once
// and returns the response instead of sleeping into a timeout.
func TestDeadlineStopsRetries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()
		s := &script{steps: []step{{status: 503,
			header: http.Header{"Retry-After": {"1"}}}}}
		resp, err := Do(ctx, &http.Client{Transport: s}, policy, NewBudget(10, 0.1), mk)
		if err != nil || resp.StatusCode != 503 || s.calls != 1 {
			t.Fatalf("resp=%v err=%v calls=%d", resp, err, s.calls)
		}
		resp.Body.Close()
	})
}

// A caller that gives up mid-wait gets ctx.Err, and the body of
// the response it will never see is closed.
func TestCancelDuringWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		s := &script{steps: []step{{status: 503,
			header: http.Header{"Retry-After": {"1"}}}}}
		go func() { time.Sleep(100 * time.Millisecond); cancel() }()
		resp, err := Do(ctx, &http.Client{Transport: s}, policy, NewBudget(10, 0.1), mk)
		if resp != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("resp=%v err=%v", resp, err)
		}
		if s.opened.Load() != s.closed.Load() {
			t.Fatal("a body was left open")
		}
	})
}

// Errors that can't succeed next time are not retried, and a bad
// policy is an error, not a panic in rand.N.
func TestNotRetried(t *testing.T) {
	var n atomic.Int32
	cl := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n.Add(1)
		return nil, errors.New("no such scheme")
	})}
	if _, err := Do(t.Context(), cl, policy, NewBudget(10, 0.1), mk); err == nil || n.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, n.Load())
	}
	for _, p := range []Policy{{}, {Attempts: 3, Base: 0, Max: time.Second},
		{Attempts: 3, Base: time.Second, Max: 0}, {Attempts: 0, Base: 1, Max: 1}} {
		if _, err := Do(t.Context(), cl, p, NewBudget(10, 0.1), mk); !errors.Is(err, errPolicy) {
			t.Errorf("%+v: err=%v", p, err)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
