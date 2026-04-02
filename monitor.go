package gleak

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Options struct {
	// Default: 10
	Threshold int

	// Default: 5 seconds
	SampleInterval time.Duration

	OnLeak func(leaked []Goroutine)
}
type Monitor struct {
	opts Options

	mu       sync.RWMutex
	baseline map[uint64]Goroutine // goroutines that existed when Start() was called
	latest   []Goroutine          // goroutines seen on the most recent check
	leaked   []Goroutine          // goroutines above threshold on the most recent check

	stop chan struct{} // closing this channel tells the background loop to exit
	done chan struct{} // closed by the background loop when it has exited
}

// NewMonitor creates a Monitor. Call Start() to begin monitoring.
func NewMonitor(opts Options) *Monitor {
	// Apply defaults.
	if opts.Threshold <= 0 {
		opts.Threshold = 10
	}
	if opts.SampleInterval <= 0 {
		opts.SampleInterval = 5 * time.Second
	}
	return &Monitor{
		opts: opts,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

func (m *Monitor) Start() {
	// Record which goroutines exist at startup — these are "expected" and will
	// not be counted as leaks.
	snap := snapshot()
	base := make(map[uint64]Goroutine, len(snap))
	for _, g := range snap {
		base[g.ID] = g
	}

	m.mu.Lock()
	m.baseline = base
	m.latest = snap
	m.mu.Unlock()

	go m.loop() // start the background checker
}
func (m *Monitor) Stop() {
	close(m.stop) // signal the loop to exit
	<-m.done      // wait until it has
}

func (m *Monitor) loop() {
	defer close(m.done) // signal Stop() that we've exited

	ticker := time.NewTicker(m.opts.SampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stop: // Stop() was called
			return
		case <-ticker.C: // time to check
			m.check()
		}
	}
}

func (m *Monitor) Snapshot() (all []Goroutine, leaked []Goroutine) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.latest, m.leaked
}

// check takes a snapshot and compares it against the baseline.
func (m *Monitor) check() {
	current := snapshot()

	m.mu.RLock()
	baseline := m.baseline
	threshold := m.opts.Threshold
	m.mu.RUnlock()

	// Collect goroutines that (a) weren't running at startup and (b) aren't
	// internal runtime goroutines.
	var newOnes []Goroutine
	for _, g := range current {
		if g.isBackground() {
			continue
		}
		if _, existedAtStart := baseline[g.ID]; existedAtStart {
			continue
		}
		newOnes = append(newOnes, g)
	}

	// Store results.
	m.mu.Lock()
	m.latest = current
	if len(newOnes) >= threshold {
		m.leaked = newOnes
	} else {
		m.leaked = nil
	}
	leaked := m.leaked
	m.mu.Unlock()

	// Fire the callback (in its own goroutine so a slow callback doesn't
	// delay the next check).
	if len(leaked) >= threshold && m.opts.OnLeak != nil {
		cp := make([]Goroutine, len(leaked))
		copy(cp, leaked)
		go m.opts.OnLeak(cp)
	}
}

// http.Handle("/debug/gleak", m.Handler())
func (m *Monitor) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		all, leaked := m.Snapshot()

		// Build a simple JSON-friendly struct (json.Marshal can't encode
		// the Goroutine type directly because of the unexported method).
		type row struct {
			ID          uint64 `json:"id"`
			State       string `json:"state"`
			TopFunction string `json:"top_function"`
			CreatedBy   string `json:"created_by,omitempty"`
			Stack       string `json:"stack"`
		}
		rows := func(gs []Goroutine) []row {
			out := make([]row, len(gs))
			for i, g := range gs {
				out[i] = row{g.ID, g.State, g.TopFunction, g.CreatedBy, g.Stack}
			}
			return out
		}

		report := struct {
			Total      int   `json:"total"`
			Leaked     int   `json:"leaked"`
			Threshold  int   `json:"threshold"`
			All        []row `json:"all"`
			LeakedList []row `json:"leaked_list"`
		}{
			Total:      len(all),
			Leaked:     len(leaked),
			Threshold:  m.opts.Threshold,
			All:        rows(all),
			LeakedList: rows(leaked),
		}

		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			http.Error(w, fmt.Sprintf("gleak encode error: %v", err), 500)
		}
	})
}
