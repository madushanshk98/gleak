package gleak_test

import "sync"

// *testing.T in tests that need to assert that a leak *was* detected
type testLeakT struct {
	mu  sync.Mutex
	msg string
}

func (f *testLeakT) Helper() {}
func (f *testLeakT) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msg = format
}
func (f *testLeakT) failed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.msg != ""
}
