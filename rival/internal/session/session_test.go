package session

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/1F47E/rival/internal/config"
)

// isolateHome points config.SessionDirPath at a per-test temp dir.
func isolateHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

func TestNew_PreviewTruncatesOnRuneBoundary(t *testing.T) {
	isolateHome(t)

	// Multi-byte runes: a byte-index slice at PromptPreviewLen would split a
	// rune and produce invalid UTF-8.
	prompt := strings.Repeat("ы", config.PromptPreviewLen+50)

	s, err := New(Opts{CLI: "codex", Mode: "raw", Model: "m", Effort: "low", WorkDir: ".", Prompt: prompt})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if !utf8.ValidString(s.PromptPreview) {
		t.Fatalf("preview is not valid UTF-8: %q", s.PromptPreview)
	}
	if got := utf8.RuneCountInString(s.PromptPreview); got != config.PromptPreviewLen {
		t.Fatalf("preview rune count = %d, want %d", got, config.PromptPreviewLen)
	}
}

func TestComplete_DoesNotOverwriteUserKill(t *testing.T) {
	isolateHome(t)

	s, err := New(Opts{CLI: "codex", Mode: "raw", Model: "m", Effort: "low", WorkDir: ".", Prompt: "hi"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Simulate the TUI killing the session.
	if err := s.Kill(137, "killed by user"); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	// The runner instance (same ID) finishes naturally and tries to complete.
	runner := &Session{ID: s.ID, Status: "running", StartTime: time.Now()}
	if err := runner.Complete(0, 100, 5); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// On-disk state must still reflect the kill, not the completion.
	loaded := loadByID(t, s.ID)
	if loaded.Status != "failed" {
		t.Fatalf("status = %q, want failed (kill must win)", loaded.Status)
	}
	if !strings.Contains(loaded.ErrorMsg, "killed") {
		t.Fatalf("error = %q, want it to retain the kill message", loaded.ErrorMsg)
	}
}

func TestFail_NonKillDoesNotOverwriteUserKill(t *testing.T) {
	isolateHome(t)

	s, err := New(Opts{CLI: "codex", Mode: "raw", Model: "m", Effort: "low", WorkDir: ".", Prompt: "hi"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Kill(137, "killed by user"); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	// A later non-kill failure (e.g. orphan reaper, or an OS "signal: killed"
	// exit message) must not clobber the user kill.
	runner := &Session{ID: s.ID, Status: "running", StartTime: time.Now()}
	if err := runner.Fail(1, "signal: killed"); err != nil {
		t.Fatalf("Fail(orphan): %v", err)
	}

	loaded := loadByID(t, s.ID)
	if !loaded.KilledByUser {
		t.Fatalf("KilledByUser = false, want the user kill preserved")
	}
	if !strings.Contains(loaded.ErrorMsg, "killed by user") {
		t.Fatalf("error = %q, want the kill message preserved", loaded.ErrorMsg)
	}
}

func loadByID(t *testing.T, id string) *Session {
	t.Helper()
	for _, s := range LoadAll() {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("session %s not found on disk", id)
	return nil
}
