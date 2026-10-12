package gh

import (
	"context"
	"testing"
)

func TestMockRunner_Strict(t *testing.T) {
	ctx := context.Background()
	lenient := &MockRunner{}
	if out, err := lenient.Run(ctx, "api", "repos/o/r"); err != nil || out != nil {
		t.Errorf("lenient: unregistered command = %q, %v; want no output and no error", out, err)
	}

	strict := &MockRunner{Strict: true, Responses: map[string][]byte{"api repos/o/r": []byte("{}")}}
	if out, err := strict.Run(ctx, "api", "repos/o/r"); err != nil || string(out) != "{}" {
		t.Errorf("strict: registered command = %q, %v", out, err)
	}
	if _, err := strict.RunWithStdin(ctx, []byte("x"), "api", "repos/o/r", "--method", "PATCH"); err == nil {
		t.Error("strict: unregistered command should fail")
	}
	if len(strict.Called) != 2 || string(strict.CalledStdin[1]) != "x" {
		t.Errorf("calls not recorded: %v %q", strict.Called, strict.CalledStdin)
	}
}
