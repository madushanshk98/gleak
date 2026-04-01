package gleak

import "runtime"

type Goroutine struct {
	ID          uint64 // the number Go assigns to every goroutine, e.g. 42
	State       string // what it's doing right now, e.g. "chan receive", "IO wait"
	TopFunction string // the function currently running inside it
	CreatedBy   string // the function that started it with the "go" keyword
	Stack       string // the full stack trace, useful for debugging
}

func snapshot() []Goroutine {
	// Start with a 64 KB buffer and double it until it fits everything.
	buf := make([]byte, 64*1024)
	for {
		n := runtime.Stack(buf, true) // true = include ALL goroutines
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, len(buf)*2) //double it until it fits everything.
	}
	return parseAllBlocks(string(buf))
}
