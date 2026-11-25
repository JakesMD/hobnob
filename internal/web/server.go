// Package web is the browser front end: a local page that observes a run. It sits
// beside tui, and like tui it never imports eval, runner or config.
package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

//go:embed page.html
var page []byte

// grace is how long hobnob waits on the person with no page open before it
// gives up. SetGrace shortens it for tests.
var grace = 60 * time.Second

// action handles one POST from the page; body is the request's JSON.
type action func(body json.RawMessage) error

// Server serves the page on 127.0.0.1 and streams the run's state to it. It
// implements tui.RunListener through its embedded state.
type Server struct {
	*state

	token    string
	listener net.Listener
	http     *http.Server
	actions  map[string]action

	choice chan string
	home   chan struct{}

	deliveredOnce sync.Once
	delivered     chan struct{}

	subMu sync.Mutex
	subs  map[*tab]struct{}
	// newest is the tab that holds the page; a newer connection replaces it.
	newest *tab

	graceMu     sync.Mutex
	grace       time.Duration
	graceTimer  *time.Timer
	graceRun    int
	abandonOnce sync.Once
	abandoned   chan struct{}
}

// tab is one open SSE connection.
type tab struct {
	changed  chan struct{}
	replaced chan struct{}
}

// Listen binds a random free port on 127.0.0.1 and makes a random token.
func Listen() (*Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		_ = listener.Close()
		return nil, err
	}
	server := &Server{
		state:     &state{},
		token:     hex.EncodeToString(raw),
		listener:  listener,
		actions:   map[string]action{},
		choice:    make(chan string, 1),
		home:      make(chan struct{}, 1),
		delivered: make(chan struct{}),
		subs:      map[*tab]struct{}{},
		abandoned: make(chan struct{}),
		grace:     grace,
	}
	server.changed = server.notify
	server.actions["choose"] = server.chooseTask
	server.actions["home"] = server.goHomeAction
	server.initCancel()
	server.actions["answer"] = server.answer
	server.actions["skip"] = server.skip
	server.actions["open"] = server.openFolder
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", server.servePage)
	mux.HandleFunc("GET /events", server.serveEvents)
	mux.HandleFunc("POST /actions/{name}", server.serveAction)
	server.http = &http.Server{Handler: server.guard(mux)}
	return server, nil
}

// URL is the address that opens the page.
func (s *Server) URL() string {
	return fmt.Sprintf("http://%s/?token=%s", s.listener.Addr(), s.token)
}

// Serve answers requests until Close; call it in its own goroutine. It also
// starts the grace timer, since no page has opened yet.
func (s *Server) Serve() {
	s.watchGrace()
	_ = s.http.Serve(s.listener)
}

// Close stops the server and ends every open stream.
func (s *Server) Close() {
	s.graceMu.Lock()
	s.stopGraceLocked()
	s.graceMu.Unlock()
	_ = s.http.Close()
}

// AwaitDelivery blocks until a page has been sent the finished run's
// snapshot, ctx ends or the person has been gone for the grace.
func (s *Server) AwaitDelivery(ctx context.Context) {
	select {
	case <-s.delivered:
	case <-ctx.Done():
		if s.hasStreams() {
			select {
			case <-s.delivered:
			case <-time.After(interruptGrace):
			}
		}
	case <-s.abandoned:
	}
}

// PickTask shows the task picker, with the last run cleared, and returns the
// name of the task the person chooses. offerHome is whether finished runs
// show Home.
func (s *Server) PickTask(ctx context.Context, tasks []Task, offerHome bool) (string, error) {
	s.startPick(tasks, offerHome)
	select {
	case name := <-s.choice:
		return name, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-s.abandoned:
		return "", errAbandoned
	}
}

var errAbandoned = errors.New("no page was open for the grace")

// AwaitHome blocks on a finished run until the person presses Home, which
// reports true. It reports false when ctx ends or no page has been open for
// the grace.
func (s *Server) AwaitHome(ctx context.Context) bool {
	select {
	case <-s.home:
		return true
	case <-ctx.Done():
		return false
	case <-s.abandoned:
		return false
	}
}

func (s *Server) chooseTask(body json.RawMessage) error {
	var req struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	if !s.choose(req.Task) {
		return fmt.Errorf("no task %q to choose", req.Task)
	}
	s.choice <- req.Task
	return nil
}

func (s *Server) goHomeAction(json.RawMessage) error {
	if !s.goHome() {
		return fmt.Errorf("home is not offered")
	}
	s.home <- struct{}{}
	return nil
}

func (s *Server) guard(next http.Handler) http.Handler {
	address := s.listener.Addr().String()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Host and Origin checks stop a web page on another site reaching
		// this server through DNS rebinding or a cross-site request.
		if r.Host != address {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+address {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(s.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) servePage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

func (s *Server) serveAction(w http.ResponseWriter, r *http.Request) {
	handle, ok := s.actions[r.PathValue("name")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	var body json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := handle(body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	mine := &tab{changed: make(chan struct{}, 1), replaced: make(chan struct{})}
	s.subMu.Lock()
	if s.newest != nil {
		delete(s.subs, s.newest)
		close(s.newest.replaced)
	}
	s.newest = mine
	s.subs[mine] = struct{}{}
	s.subMu.Unlock()
	s.watchGrace()
	defer func() {
		s.subMu.Lock()
		delete(s.subs, mine)
		if s.newest == mine {
			s.newest = nil
		}
		s.subMu.Unlock()
		s.watchGrace()
	}()
	for {
		snap, _ := s.snapshot()
		data, _ := json.Marshal(snap)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
		if snap.Result != nil {
			s.deliveredOnce.Do(func() { close(s.delivered) })
		}
		select {
		case <-mine.changed:
		case <-mine.replaced:
			data, _ := json.Marshal(Snapshot{Screen: ScreenAnotherTab})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return
		case <-r.Context().Done():
			return
		}
	}
}

// notify wakes every open stream to send the state as it now is; a stream
// that is behind gets one snapshot, the latest.
func (s *Server) notify() {
	s.subMu.Lock()
	for open := range s.subs {
		select {
		case open.changed <- struct{}{}:
		default:
		}
	}
	s.subMu.Unlock()
	s.watchGrace()
}
