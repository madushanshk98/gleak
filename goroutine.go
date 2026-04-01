package gleak

import (
	"runtime"
	"strconv"
	"strings"
)


//Sample go
// goroutine 18 [chan receive]:
// main.worker(0xc000056780)
//         /home/user/main.go:42 +0x58
// created by main.main in goroutine 1
//         /home/user/main.go:10 +0x2c
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

func parseOneBlock(block string) (Goroutine, bool) {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	if len(lines) < 2 {
		return Goroutine{}, false
	}

	// ── Parse the header line ────────────────────────────────────────────────
	// Example: "goroutine 18 [chan receive]:"
	header := lines[0]
	if !strings.HasPrefix(header, "goroutine ") {
		return Goroutine{}, false
	}
	header = strings.TrimPrefix(header, "goroutine ")
	header = strings.TrimSuffix(header, ":")

	// Split "18 [chan receive]" into id="18" and state="chan receive"
	bracketAt := strings.Index(header, " [")
	if bracketAt < 0 {
		return Goroutine{}, false
	}
	id, err := strconv.ParseUint(header[:bracketAt], 10, 64)
	if err != nil {
		return Goroutine{}, false
	}
	state := strings.Trim(header[bracketAt:], " []:")

	// ── Parse the top function (second line) ─────────────────────────────────
	// Example: "main.worker(0xc000056780)"  →  "main.worker"
	topFunc := ""
	if len(lines) > 1 {
		topFunc = funcNameOnly(lines[1])
	}

	// ── Find the "created by" line (scan from the bottom) ────────────────────
	// Example: "created by main.main in goroutine 1"  →  "main.main"
	createdBy := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "created by ") {
			raw := strings.TrimPrefix(lines[i], "created by ")
			// Go 1.21+ adds " in goroutine N" — strip it.
			if idx := strings.Index(raw, " in goroutine "); idx >= 0 {
				raw = raw[:idx]
			}
			createdBy = strings.TrimSpace(raw)
			break
		}
	}

	return Goroutine{
		ID:          id,
		State:       state,
		TopFunction: topFunc,
		CreatedBy:   createdBy,
		Stack:       block,
	}, true
}
