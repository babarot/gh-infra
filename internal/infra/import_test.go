package infra

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/babarot/gh-infra/internal/gh"
)

// labelsForbidden answers as importRunner does, except that the labels of
// o/a cannot be read.
type labelsForbidden struct{ importRunner }

func (r *labelsForbidden) Run(ctx context.Context, args ...string) ([]byte, error) {
	if strings.HasPrefix(strings.Join(args, " "), "label list --repo o/a") {
		return nil, gh.ErrForbidden
	}
	return r.importRunner.Run(ctx, args...)
}

// captureOutput runs f with os.Stdout and os.Stderr captured.
func captureOutput(t *testing.T, f func()) (stdout, stderr string) {
	t.Helper()
	read := func(r *os.File, into *string, done chan<- struct{}) {
		b, _ := io.ReadAll(r)
		*into = string(b)
		close(done)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	outDone, errDone := make(chan struct{}), make(chan struct{})
	go read(outR, &stdout, outDone)
	go read(errR, &stderr, errDone)
	defer func() {
		os.Stdout, os.Stderr = oldOut, oldErr
	}()
	f()
	_ = outW.Close()
	_ = errW.Close()
	<-outDone
	<-errDone
	return stdout, stderr
}

// import says on stderr what it could not read and left out of the YAML,
// which cannot tell it from what GitHub does not have.
func TestImportWarnsAboutUnread(t *testing.T) {
	var runErr error
	stdout, stderr := captureOutput(t, func() {
		runErr = Import(context.Background(), &labelsForbidden{}, []string{"o/a", "o/b"})
	})
	if runErr != nil {
		t.Fatal(runErr)
	}
	if !strings.Contains(stderr, "o/a  labels could not be read and are left out") {
		t.Errorf("no warning for o/a on stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, "o/b  ") {
		t.Errorf("a warning for o/b, which was read:\n%s", stderr)
	}
	if strings.Contains(stdout, "could not be read") {
		t.Errorf("the warning is in the YAML on stdout:\n%s", stdout)
	}
	if strings.Count(stdout, "kind: Repository") != 2 {
		t.Errorf("want both repositories exported:\n%s", stdout)
	}
}
