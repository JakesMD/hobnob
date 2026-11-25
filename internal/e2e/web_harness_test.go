package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"hobnob/internal/web"
)

// browserTimeout bounds every wait of a played browser, so a snapshot that
// never comes fails the test instead of hanging it.
const browserTimeout = 15 * time.Second

// Browser plays the browser a --web run opened: it subscribes to the page's
// SSE stream, reads its snapshots and POSTs its actions. A failed check ends
// the goroutine it runs in, which cancels the run.
type Browser struct {
	t     *testing.T
	base  string
	token string
	dir   string
}

func newBrowser(t *testing.T, opened, dir string) *Browser {
	t.Helper()
	parsed, err := url.Parse(opened)
	if err != nil {
		t.Errorf("OpenBrowser got %q: %v", opened, err)
		runtime.Goexit()
	}
	return &Browser{t: t, base: parsed.Scheme + "://" + parsed.Host, token: parsed.Query().Get("token"), dir: dir}
}

// Fail ends the played browser with a failed test.
func (b *Browser) Fail(format string, args ...any) {
	b.t.Helper()
	b.fail(format, args...)
}

func (b *Browser) fail(format string, args ...any) {
	b.t.Helper()
	b.t.Errorf(format, args...)
	runtime.Goexit()
}

// Address is the host and port the page was opened on.
func (b *Browser) Address() string { return strings.TrimPrefix(b.base, "http://") }

// Release creates the file in the run's directory that a fixture's command is
// waiting for.
func (b *Browser) Release(name string) {
	b.t.Helper()
	if err := os.WriteFile(filepath.Join(b.dir, name), nil, 0o644); err != nil {
		b.fail("release %s: %v", name, err)
	}
}

// Get requests path with the given token (the right one is b's own) and
// returns the status and body.
func (b *Browser) Get(path, token string) (int, string) {
	b.t.Helper()
	return b.do(http.MethodGet, path, token, nil)
}

// Post sends the named action with body as JSON, with the right token.
func (b *Browser) Post(action string, body any) int {
	b.t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		b.fail("marshal %v: %v", body, err)
	}
	status, _ := b.do(http.MethodPost, "/actions/"+action, b.token, data)
	return status
}

// Token is the secret the page was opened with.
func (b *Browser) Token() string { return b.token }

func (b *Browser) do(method, path, token string, body []byte) (int, string) {
	b.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
	defer cancel()
	target := b.base + path
	if token != "" {
		target += "?token=" + url.QueryEscape(token)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		b.fail("%s %s: %v", method, path, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.fail("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var text strings.Builder
	_, _ = bufio.NewReader(resp.Body).WriteTo(&text)
	return resp.StatusCode, text.String()
}

// Tab is one open page: an SSE subscription.
type Tab struct {
	b      *Browser
	cancel context.CancelFunc
	events *bufio.Scanner
	body   interface{ Close() error }
}

// Open subscribes to the page's stream, as a tab loading the page would.
func (b *Browser) Open() *Tab {
	b.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.base+"/events?token="+url.QueryEscape(b.token), nil)
	if err != nil {
		cancel()
		b.fail("open tab: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		b.fail("open tab: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		resp.Body.Close()
		b.fail("open tab: status %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(nil, 1<<20)
	return &Tab{b: b, cancel: cancel, events: scanner, body: resp.Body}
}

// Close drops the connection, as closing the tab does.
func (t *Tab) Close() {
	t.cancel()
	_ = t.body.Close()
}

// Until reads snapshots until one satisfies done, and returns it.
func (t *Tab) Until(done func(web.Snapshot) bool) web.Snapshot {
	t.b.t.Helper()
	var last web.Snapshot
	for t.events.Scan() {
		data, ok := strings.CutPrefix(t.events.Text(), "data: ")
		if !ok {
			continue
		}
		var snap web.Snapshot
		if err := json.Unmarshal([]byte(data), &snap); err != nil {
			t.b.fail("bad snapshot %q: %v", data, err)
		}
		if last = snap; done(snap) {
			return snap
		}
	}
	t.b.fail("stream ended before the wanted snapshot; last one: %+v", last)
	return last
}

// Screen reads snapshots until one shows screen.
func (t *Tab) Screen(screen string) web.Snapshot {
	t.b.t.Helper()
	return t.Until(func(s web.Snapshot) bool { return s.Screen == screen })
}
