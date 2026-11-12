//go:build !linux && !darwin

package runner

// waitExitUnreaped is unsupported here: the caller reaps first, and so can't
// safely signal the step's process group afterwards.
func waitExitUnreaped(pid int) bool { return false }
