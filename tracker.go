package gleak

import "sync"

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
