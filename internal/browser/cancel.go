package browser

import (
	"encoding/json"
	"errors"
	"time"
)

// SetInterrupt tells the server how to stop the run that starts now: stop is
// a first CTRL+C, the graceful stop, and kill is a second one, which
// force-stops a step that ignores it. Cancel acts only between SetInterrupt
// and ClearInterrupt.
func (s *Server) SetInterrupt(stop, kill func()) {
	s.mu.Lock()
	s.stop, s.kill = stop, kill
	s.mu.Unlock()
}

// ClearInterrupt ends the run SetInterrupt began, so a Cancel that arrives
// later has nothing to stop.
func (s *Server) ClearInterrupt() {
	s.mu.Lock()
	s.stop, s.kill = nil, nil
	s.mu.Unlock()
}

// Cancel presses Cancel as the page does: the first call stops the run
// gracefully and marks it as stopping, every later call force-stops. It does
// nothing, and says false, unless a run is live.
func (s *Server) Cancel() bool {
	s.mu.Lock()
	stop, kill := s.stop, s.kill
	live := stop != nil && s.result == nil
	first := live && !s.cancelling
	if first {
		s.cancelling = true
		s.version++
	}
	s.mu.Unlock()
	switch {
	case !live:
	case first:
		if s.changed != nil {
			s.changed()
		}
		stop()
	default:
		kill()
	}
	return live
}

func (s *Server) initCancel() {
	s.actions["cancel"] = func(json.RawMessage) error {
		if !s.Cancel() {
			return errors.New("nothing to cancel")
		}
		return nil
	}
}

// watchGrace starts or stops the grace timer to match the state: it runs
// while no page is open and hobnob waits on the person (the task picker, a
// prompt, Done or Failed), never while steps run, and never on a fixed port.
func (s *Server) watchGrace() {
	s.graceMu.Lock()
	defer s.graceMu.Unlock()
	waiting := s.waitingOnPerson()
	if waiting && !s.fixed && !s.hasStreams() {
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
	if atPrompt {
		s.Cancel()
	}
	s.abandonOnce.Do(func() { close(s.abandoned) })
}

func (s *Server) hasStreams() bool {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	return len(s.subs) > 0
}
