package web

import (
	"encoding/json"
	"errors"
	"time"
)

// interruptGrace is how long a run cut short by CTRL+C in the terminal waits
// for an open page to take the Cancelled snapshot before hobnob exits.
const interruptGrace = 2 * time.Second

// SetInterrupt tells the server how to stop the run: stop is a first CTRL+C,
// the graceful stop, and kill is a second one, which force-stops a step that
// ignores it. Call it before Serve.
func (s *Server) SetInterrupt(stop, kill func()) {
	s.stop, s.kill = stop, kill
}

// Cancel presses Cancel as the page does: the first call stops the run
// gracefully and marks it as stopping, every later call force-stops. It does
// nothing once the run has ended.
func (s *Server) Cancel() {
	first, live := false, false
	s.update(func() {
		live = s.result == nil
		first = live && !s.cancelling
		if live {
			s.cancelling = true
		}
	})
	switch {
	case !live:
	case first:
		s.stop()
	default:
		s.kill()
	}
}

func (s *Server) initCancel() {
	s.actions["cancel"] = func(json.RawMessage) error {
		if s.stop == nil || s.kill == nil {
			return errors.New("nothing to cancel")
		}
		s.Cancel()
		return nil
	}
}

// watchGrace starts or stops the grace timer to match the state: it runs
// while no page is open and hobnob waits on the person (the task picker, a
// prompt, Done or Failed), never while steps run.
func (s *Server) watchGrace() {
	s.graceMu.Lock()
	defer s.graceMu.Unlock()
	waiting := s.waitingOnPerson()
	if waiting && !s.hasStreams() {
		if s.graceTimer == nil {
			s.graceRun++
			run := s.graceRun
			s.graceTimer = time.AfterFunc(s.grace, func() { s.giveUp(run) })
		}
		return
	}
	s.stopGraceLocked()
}

func (s *Server) stopGraceLocked() {
	if s.graceTimer != nil {
		s.graceTimer.Stop()
		s.graceTimer = nil
		s.graceRun++
	}
}

func (s *Server) waitingOnPerson() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.picking || s.prompt != nil || s.result != nil
}

// giveUp is the grace running out: at a prompt it cancels as a first CTRL+C
// does, and either way it releases everything waiting on the page.
func (s *Server) giveUp(run int) {
	s.graceMu.Lock()
	current := s.graceTimer != nil && s.graceRun == run
	s.graceTimer = nil
	s.graceMu.Unlock()
	if !current {
		return
	}
	s.mu.Lock()
	atPrompt := s.prompt != nil
	s.mu.Unlock()
	if atPrompt && s.stop != nil {
		s.Cancel()
	}
	s.abandonOnce.Do(func() { close(s.abandoned) })
}

func (s *Server) hasStreams() bool {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	return len(s.subs) > 0
}
