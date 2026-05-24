package executor

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

// GeminiPreflight checks that the agy (Antigravity) CLI is installed.
// rival drives Gemini 3.5 Flash through agy; the legacy gemini CLI is no
// longer used because its backend does not serve the 3.5-flash model.
func GeminiPreflight() error {
	if _, err := exec.LookPath("agy"); err != nil {
		return fmt.Errorf("agy CLI not installed. Install the Antigravity CLI (agy) and sign in")
	}
	return nil
}

// RunGemini executes a prompt through the Antigravity CLI (agy), which runs
// Gemini 3.5 Flash. agy -p is an agentic print mode: it returns a single
// response non-interactively. --sandbox auto-approves read-only tool calls so
// the reviewer never blocks on a permission prompt. The prompt is delivered on
// stdin by RunSubprocess; effort has no agy equivalent and is intentionally
// ignored.
func RunGemini(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	args := []string{
		"-p",
		"--sandbox",
	}

	// Pass nil for extra env: RunSubprocess seeds the child environment from
	// safeEnv() (filtered os.Environ()). Passing os.Environ() here would
	// re-append the unfiltered environment and let blocked vars slip back in.
	fullPrompt := config.SystemPrompt + "\n\n" + prompt
	return RunSubprocess(ctx, sess, "agy", args, nil, fullPrompt, mirror)
}
