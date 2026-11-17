package runner

import (
	"sync"
	"time"
)

// A leftover is what a run: step left running once its shell exited: a
// backgrounded job still in the step's process group, or still holding its
// output pipes. It keeps running, and its output keeps showing under the
// step's prefix, until the run ends and stopLeftovers stops it.
type leftover struct {
	// group is the step's process group, led by its shell, which stays
	// unreaped so the group ID can't be reused while hobnob may signal it.
	// 0 once the shell is reaped, or when it was reaped before the group
	// could be checked.
	group    int
	termSent bool // a first CTRL+C already SIGTERMed the group
	reap     func() error
	pipes    *stepPipes
	flush    func()
}

// leftoversMu guards leftovers and every leftover's group, so a 2nd CTRL+C
// (KillRunningStep) never signals a group whose shell has been reaped.
var leftoversMu sync.Mutex
var leftovers []*leftover

// keepLeftover registers l, first letting go of earlier leftovers that are
// done: an empty group's shell is reaped, and once its pipes are closed too,
// the leftover is dropped.
func keepLeftover(l *leftover) {
	leftoversMu.Lock()
	defer leftoversMu.Unlock()
	kept := leftovers[:0]
	for _, old := range leftovers {
		if old.group != 0 && !groupHasOthers(old.group) {
			old.reap()
			old.group = 0
		}
		if old.group == 0 && old.pipes.closed() {
			old.pipes.withLock(old.flush)
			continue
		}
		kept = append(kept, old)
	}
	leftovers = append(kept, l)
}

// stopLeftovers ends the run's leftovers: SIGTERM, up to leftoverGrace for
// the groups to empty, then SIGKILL. Output drains for killDrain more, then
// the pipes are abandoned and the shells reaped.
func stopLeftovers() {
	termLeftovers()
	leftoversMu.Lock()
	all := leftovers
	leftoversMu.Unlock()
	if len(all) == 0 {
		return
	}
	deadline := time.Now().Add(leftoverGrace)
	for leftoversAlive(all) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	killLeftovers()
	drainUntil := time.Now().Add(killDrain)
	for _, l := range all {
		l.pipes.waitClosed(time.Until(drainUntil))
	}
	leftoversMu.Lock()
	defer leftoversMu.Unlock()
	for _, l := range all {
		l.pipes.drop()
		l.pipes.withLock(l.flush)
		if l.group != 0 {
			l.reap()
			l.group = 0
		}
	}
	leftovers = nil
}

func leftoversAlive(all []*leftover) bool {
	leftoversMu.Lock()
	defer leftoversMu.Unlock()
	for _, l := range all {
		if l.group != 0 && groupHasOthers(l.group) {
			return true
		}
	}
	return false
}

// killLeftovers SIGKILLs every leftover group still registered.
func killLeftovers() {
	leftoversMu.Lock()
	defer leftoversMu.Unlock()
	for _, l := range leftovers {
		if l.group != 0 {
			signalGroupKill(l.group)
		}
	}
}

// termLeftovers SIGTERMs every leftover group not yet signalled. It runs on
// a first CTRL+C, so earlier steps' jobs start stopping at once instead of
// only once the running step has ended.
func termLeftovers() {
	leftoversMu.Lock()
	defer leftoversMu.Unlock()
	for _, l := range leftovers {
		if l.group != 0 && !l.termSent {
			signalGroupTerm(l.group)
			l.termSent = true
		}
	}
}
