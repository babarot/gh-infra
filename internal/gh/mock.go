package gh

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MockRunner is a test double for Runner.
// Called[i] and CalledStdin[i] always correspond to the same call.
// CalledStdin[i] is nil for Run calls and contains the body for RunWithStdin calls.
// Concurrent callers are safe: Called/CalledStdin are guarded by mu.
//
// A command with no registered response or error succeeds with no output,
// unless Strict is set: then it fails, so a test that calls a command it did
// not expect (a wrong endpoint or argument) does not pass silently.
type MockRunner struct {
	Responses   map[string][]byte
	Errors      map[string]error
	Called      [][]string
	CalledStdin [][]byte
	Strict      bool

	mu sync.Mutex
}

func (m *MockRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	return m.call(nil, args)
}

func (m *MockRunner) RunWithStdin(_ context.Context, body []byte, args ...string) ([]byte, error) {
	return m.call(body, args)
}

func (m *MockRunner) call(body []byte, args []string) ([]byte, error) {
	key := strings.Join(args, " ")
	m.mu.Lock()
	m.Called = append(m.Called, args)
	m.CalledStdin = append(m.CalledStdin, body)
	m.mu.Unlock()
	if err, ok := m.Errors[key]; ok {
		return nil, err
	}
	if resp, ok := m.Responses[key]; ok {
		return resp, nil
	}
	if m.Strict {
		return nil, fmt.Errorf("unexpected gh command: %s", key)
	}
	return nil, nil
}
