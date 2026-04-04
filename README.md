# gleak — goroutine leak detector for Go

Zero-dependency. Works in production **and** in tests.

---

## The problem it solves

A "goroutine leak" happens when your code starts a goroutine with `go func()`
but that goroutine never stops running — even after the work it was meant to do
is finished. Over time, hundreds or thousands of these pile up and slow your
program down.

`gleak` detects this in two places:

| Where | Tool | What it does |
|---|---|---|
| **Tests** | `Tracker` | Fails the test if any new goroutines are still running when the test ends |
| **Production** | `Monitor` | Alerts you via callback + HTTP when goroutines pile up above a threshold |

---

## Install

```bash
go get github.com/madushanshk98/gleak
```

---

## Use in tests (Tracker)

```go
func TestMyFeature(t *testing.T) {
    // 1. Create a tracker at the top of the test.
    tk := gleak.NewTracker()

    // 2. Defer VerifyNone — it runs when the test function returns.
    defer tk.VerifyNone(t)

    // 3. Run your code normally.
    go myBackgroundTask() // if this never stops, the test will fail
}
```

### Spawning goroutines you want to track

```go
tk.Go(func() {
    // This goroutine MUST exit before the test ends.
    doWork()
})
```

### Ignoring known long-lived goroutines

```go
tk.Ignore(
    "database/sql",               // ignore DB connection pool goroutines
    "net/http.(*persistConn)",    // ignore keep-alive HTTP goroutines
)
```

### Why is this better than goleak?

- `t.Parallel()` works correctly — each tracker has its own baseline
- Built-in retry window (100 ms) avoids flaky failures from shutdown races
- Zero dependencies

---

## Use in production (Monitor)

```go
func main() {
    m := gleak.NewMonitor(gleak.Options{
        Threshold:      20,               // alert when 20+ new goroutines appear
        SampleInterval: 10 * time.Second, // check every 10 seconds
        OnLeak: func(leaked []gleak.Goroutine) {
            for _, g := range leaked {
                log.Printf("leaked goroutine %d [%s] spawned by %s",
                    g.ID, g.State, g.CreatedBy)
            }
        },
    })
    m.Start()
    defer m.Stop()

    // Expose a live JSON inspector at /debug/gleak
    http.Handle("/debug/gleak", m.Handler())

    http.ListenAndServe(":8080", nil)
}
```

### /debug/gleak JSON response

```json
{
  "total": 14,
  "leaked": 3,
  "threshold": 20,
  "leaked_list": [
    {
      "id": 42,
      "state": "chan receive",
      "top_function": "main.worker",
      "created_by": "main.startPool",
      "stack": "goroutine 42 [chan receive]:\n..."
    }
  ]
}
```

### Checking the snapshot programmatically

```go
all, leaked := m.Snapshot()
fmt.Printf("total goroutines: %d, leaked: %d\n", len(all), len(leaked))
```

---

## The Goroutine type

Both `Monitor` and `Tracker` give you `[]gleak.Goroutine` values:

```go
type Goroutine struct {
    ID          uint64 // e.g. 42
    State       string // e.g. "chan receive", "IO wait", "select"
    TopFunction string // the function currently running inside it
    CreatedBy   string // the function that started it with "go"
    Stack       string // full stack trace for debugging
}
```

---

## License

MIT
