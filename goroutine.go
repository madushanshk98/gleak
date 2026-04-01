package gleak

import (
	"runtime"
	"strings"
)

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

func parseAllBlocks(dump string) []Goroutine {
	var result []Goroutine
	var current strings.Builder

	for _, line := range strings.Split(dump, "\n") {
		if strings.TrimSpace(line) == "" {
			// Blank line = end of one goroutine block.
			if current.Len() > 0 {
				if g, ok := parseOneBlock(current.String()); ok {
					result = append(result, g)
				}
				current.Reset()
			}
		} else {
			current.WriteString(line)
			current.WriteByte('\n')
		}
	}
	// Handle the last block if the dump doesn't end with a blank line.
	if current.Len() > 0 {
		if g, ok := parseOneBlock(current.String()); ok {
			result = append(result, g)
		}
	}
	return result
}
