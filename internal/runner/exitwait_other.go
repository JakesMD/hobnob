//go:build !linux && !darwin

package runner

// waitExitUnreaped is unsupported here: the caller reaps first, and so can't
// safely signal the step's process group afterwards.
func waitExitUnreaped(pid int) bool { return false }

// shellStatus and groupHasOthers are never reached here, since
// waitExitUnreaped never leaves a shell unreaped.
func shellStatus(pid int) (error, bool) { return nil, false }
func groupHasOthers(pgid int) bool      { return false }
