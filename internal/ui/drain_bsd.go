//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package ui

import (
	"os"

	"golang.org/x/sys/unix"
)

// drainStdinAfterBubbletea flushes pending terminal input that leaks as
// garbage characters (e.g. ^[[?2026;2$y, ^[[?2027;2$y, ^[[?1u) after a
// bubbletea v2 program exits.
//
// Bug details:
// bubbletea v2's cursed_renderer sends terminal capability queries on startup:
//   - DECRQM for Synchronized Output (mode 2026)
//   - DECRQM for Unicode Core (mode 2027)
//   - Kitty Keyboard Protocol query (?1u response)
//
// These queries are written to the output (stderr/stdout), and the terminal
// asynchronously responds via stdin. However, during shutdown bubbletea calls
// cancelReader.Cancel() to stop the input read loop *before* stopRenderer()
// flushes the final mode-reset sequences. This means the terminal's query
// responses arrive after the read loop has exited and nobody consumes them,
// so they leak into the user's shell prompt.
//
// This drain approach is borrowed from a community fork:
//
//	https://github.com/saltydk/bubbletea/commit/96c1e05
//
// Related upstream issues (open as of bubbletea v2.0.2, no official fix yet):
//   - https://github.com/charmbracelet/bubbletea/issues/1590
//   - https://github.com/charmbracelet/bubbletea/issues/1627
//
// TODO: Remove this workaround once bubbletea ships an official fix.
func drainStdinAfterBubbletea() {
	f := os.Stdin
	if f == nil {
		return
	}
	// TIOCFLUSH with flag 1 (FREAD) discards data received but not yet read.
	_ = unix.IoctlSetPointerInt(int(f.Fd()), unix.TIOCFLUSH, 1)
}
