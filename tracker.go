package gleak

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
}
type Tracker struct {
	mu          sync.Mutex
	baseline    map[uint64]Goroutine // goroutines running when NewTracker() was called
	ignoreNames []string             // function name prefixes to ignore
}

// Call NewTracker() AFTER t.Parallel() if you use that.
func NewTracker() *Tracker {
	snap := snapshot()
	base := make(map[uint64]Goroutine, len(snap))
	for _, g := range snap {
		base[g.ID] = g
	}
	return &Tracker{baseline: base}
}

// tk := gleak.NewTracker()
// tk.Ignore("database/sql", "net/http.(*persistConn)")
// defer tk.VerifyNone(t)
func (tk *Tracker) Ignore(prefixes ...string) {
	tk.mu.Lock()
	defer tk.mu.Unlock()
	tk.ignoreNames = append(tk.ignoreNames, prefixes...)
}

func (tk *Tracker) Go(fn func()) {
	// We use a small channel to get the goroutine's ID the moment it starts,
	// before fn() runs, so we can track it properly.
	started := make(chan uint64, 1)
	go func() {
		started <- selfID() // send our own goroutine ID back
		fn()
	}()
	<-started // wait until the goroutine has started (and sent its ID)

}

// defer tk.VerifyNone(t)
func (tk *Tracker) VerifyNone(t TestingT) {
	t.Helper()

	// Retry loop — give leaking goroutines up to 100 ms to exit on their own.
	var leaks []Goroutine
	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		leaks = tk.findLeaks()
		if len(leaks) == 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if len(leaks) == 0 {
		return // all good!
	}

	// Build a human-readable error message.
	t.Helper()
	var msg strings.Builder
	fmt.Fprintf(&msg, "gleak: %d goroutine(s) leaked:\n", len(leaks))
	for _, g := range leaks {
		fmt.Fprintf(&msg, "\n  goroutine %d [%s]\n", g.ID, g.State)
		if g.CreatedBy != "" {
			fmt.Fprintf(&msg, "  spawned by: %s\n", g.CreatedBy)
		}
		fmt.Fprintf(&msg, "  running:    %s\n", g.TopFunction)
		// Indent the full stack trace for readability.
		for _, line := range strings.Split(g.Stack, "\n") {
			if line != "" {
				fmt.Fprintf(&msg, "    %s\n", line)
			}
		}
	}
	t.Errorf("%s", msg.String())
}

// findLeaks compares the current goroutines against the baseline and the
// user-supplied ignore list.
func (tk *Tracker) findLeaks() []Goroutine {
	tk.mu.Lock()
	ignoreNames := tk.ignoreNames
	tk.mu.Unlock()

	current := snapshot()

	var leaks []Goroutine
	for _, g := range current {
		// Skip Go runtime internals.
		if g.isBackground() {
			continue
		}
		// Skip goroutines that already existed before this test started.
		if _, wasBaseline := tk.baseline[g.ID]; wasBaseline {
			continue
		}
		// Skip goroutines the user asked us to ignore.
		if matchesAny(g, ignoreNames) {
			continue
		}
		leaks = append(leaks, g)
	}
	return leaks
}

// matchesAny returns true if the goroutine's top function or creator matches
// any of the given prefixes.
func matchesAny(g Goroutine, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(g.TopFunction, p) {
			return true
		}
		if strings.HasPrefix(g.CreatedBy, p) {
			return true
		}
	}
	return false
}
