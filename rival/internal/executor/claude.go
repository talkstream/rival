package executor

import (
	"context"
	"io"
	"os/exec"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

// ClaudePreflight checks that claude is available (native or docker).
func ClaudePreflight() error {
	if _, err := exec.LookPath("claude"); err == nil {
		return nil
	}
	return ClaudeDockerPreflight()
}

// RunClaude executes a prompt through Claude CLI (native if available, docker otherwise).
func RunClaude(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	if _, err := exec.LookPath("claude"); err == nil {
		sess.Mode = "native"
		return runClaudeNative(ctx, sess, prompt, effort, workdir, mirror)
	}
	sess.Mode = "docker"
	return RunClaudeDocker(ctx, sess, prompt, effort, workdir, mirror)
}

func runClaudeNative(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	claudeEffort := config.ClaudeEffortLevel[effort]
	if claudeEffort == "" {
		claudeEffort = "max"
	}

	args := []string{
		"-p",
		"--model", config.ClaudeModel,
		"--effort", claudeEffort,
		"--output-format", "text",
		"--no-session-persistence",
		"--dangerously-skip-permissions",
		"--system-prompt", config.SystemPrompt,
	}

	// Pass nil so RunSubprocess applies safeEnv() only. Passing os.Environ() here
	// is appended AFTER safeEnv() inside RunSubprocess, which (per Go's cmd.Env
	// last-wins semantics) defeats the safeEnv filter for HTTP_PROXY, DYLD_*,
	// NODE_OPTIONS, and LD_PRELOAD. Codex, Gemini, and Docker executors all pass
	// nil — this aligns the native Claude executor with them.
	return RunSubprocess(ctx, sess, "claude", args, nil, prompt, mirror)
}
