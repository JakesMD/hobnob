package web

import "time"

// SetGrace shortens how long hobnob waits on the person with no page open,
// and returns a func that restores it. Test-only seam.
func SetGrace(d time.Duration) (restore func()) {
	orig := grace
	grace = d
	return func() { grace = orig }
}
