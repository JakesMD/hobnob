package runevents

import "time"

// Nop is a Listener that hears everything and does nothing; embed it and
// override the events to show.
type Nop struct{}

func (Nop) TaskEntered(string, string)                           {}
func (Nop) TaskLeft(string, bool)                                {}
func (Nop) TaskSkipped(string)                                   {}
func (Nop) TaskDone(string, time.Duration, int)                  {}
func (Nop) TaskFailed(string, time.Duration, bool)               {}
func (Nop) Stopping(string)                                      {}
func (Nop) StepStarted(string, string, []string)                 {}
func (Nop) StepHeld(string)                                      {}
func (Nop) OutputLine(string, string)                            {}
func (Nop) StepSkipped(string, string)                           {}
func (Nop) StepDone(string, time.Duration)                       {}
func (Nop) StepFailed(string, time.Duration, string, string)     {}
func (Nop) StepSoftFailed(string, time.Duration, string, string) {}
func (Nop) StepError(string, bool, string)                       {}
func (Nop) Call(string, string)                                  {}
func (Nop) DoneEarlier(string)                                   {}
func (Nop) Var(string, string, string, string)                   {}
func (Nop) VarSkipped(string, string)                            {}
func (Nop) Say(string, string)                                   {}
func (Nop) Finished(Outcome)                                     {}
