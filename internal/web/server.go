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
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

//go:embed page.html
var page []byte

// deliveryGrace is how long a finished run waits for a page to take the
// final snapshot before hobnob gives up on it.
const deliveryGrace = 60 * time.Second

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

	deliveredOnce sync.Once
	delivered     chan struct{}

	subMu sync.Mutex
	subs  map[chan struct{}]struct{}
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
		delivered: make(chan struct{}),
		subs:      map[chan struct{}]struct{}{},
	}
	server.changed = server.notify
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

// Serve answers requests until Close; call it in its own goroutine.
func (s *Server) Serve() { _ = s.http.Serve(s.listener) }

// Close stops the server and ends every open stream.
func (s *Server) Close() { _ = s.http.Close() }

// AwaitDelivery blocks until a page has been sent the finished run's
// snapshot, ctx ends or the grace runs out.
func (s *Server) AwaitDelivery(ctx context.Context) {
	select {
	case <-s.delivered:
	case <-ctx.Done():
	case <-time.After(deliveryGrace):
	}
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
	changed := make(chan struct{}, 1)
	s.subMu.Lock()
	s.subs[changed] = struct{}{}
	s.subMu.Unlock()
	defer func() {
		s.subMu.Lock()
		delete(s.subs, changed)
		s.subMu.Unlock()
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
		case <-changed:
		case <-r.Context().Done():
			return
		}
	}
}

// notify wakes every open stream to send the state as it now is; a stream
// that is behind gets one snapshot, the latest.
func (s *Server) notify() {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for changed := range s.subs {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
}
