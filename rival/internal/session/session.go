package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/1F47E/rival/internal/config"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type Session struct {
	ID            string     `json:"id"`
	GroupID       string     `json:"group_id,omitempty"`
	CLI           string     `json:"cli"`
	Mode          string     `json:"mode"`
	Model         string     `json:"model"`
	Effort        string     `json:"effort"`
	ReviewScope   string     `json:"review_scope,omitempty"`
	Prompt        string     `json:"prompt,omitempty"`
	PromptPreview string     `json:"prompt_preview,omitempty"`
	PromptHash    string     `json:"prompt_hash,omitempty"`
	Status        string     `json:"status"`
	StartTime     time.Time  `json:"start_time"`
	EndTime       *time.Time `json:"end_time,omitempty"`
	ExitCode      *int       `json:"exit_code,omitempty"`
	Duration      string     `json:"duration,omitempty"`
	WorkDir       string     `json:"work_dir"`
	LogFile       string     `json:"log_file"`
	OutputBytes   int64      `json:"output_bytes"`
	OutputLines   int        `json:"output_lines"`
	ErrorMsg      string     `json:"error,omitempty"`
	Account       string     `json:"account,omitempty"`
	PID           int        `json:"pid"`
	// KilledByUser marks a session terminated by an explicit user action (TUI).
	// It is a final state: the runner's own Complete/Fail must not overwrite it.
	KilledByUser bool `json:"killed_by_user,omitempty"`
}

// Opts holds the fields needed to create a new session. Using a struct instead
// of positional parameters keeps call sites self-documenting and immune to
// silently swapping two same-typed arguments.
type Opts struct {
	CLI         string
	Mode        string
	Model       string
	Effort      string
	WorkDir     string
	Prompt      string
	ReviewScope string
	GroupID     string // links sessions that belong together (e.g. megareview); "" for standalone
}

// New creates a new session and writes the initial JSON file.
func New(opts Opts) (*Session, error) {
	dir := config.SessionDirPath()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	id := uuid.New().String()
	logFile := filepath.Join(dir, id+".log")

	// Truncate on a rune boundary so multi-byte UTF-8 (emoji, CJK) is never
	// split mid-character, which would produce invalid UTF-8 in the JSON.
	preview := opts.Prompt
	if utf8.RuneCountInString(preview) > config.PromptPreviewLen {
		preview = string([]rune(preview)[:config.PromptPreviewLen])
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(opts.Prompt)))

	s := &Session{
		ID:            id,
		GroupID:       opts.GroupID,
		CLI:           opts.CLI,
		Mode:          opts.Mode,
		Model:         opts.Model,
		Effort:        opts.Effort,
		ReviewScope:   opts.ReviewScope,
		Prompt:        opts.Prompt,
		PromptPreview: preview,
		PromptHash:    hash,
		Status:        "running",
		StartTime:     time.Now(),
		WorkDir:       opts.WorkDir,
		LogFile:       logFile,
	}

	if err := s.Save(); err != nil {
		return nil, err
	}
	return s, nil
}

// Save writes the session JSON atomically (tmp file + rename).
func (s *Session) Save() error {
	dir := config.SessionDirPath()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	final := filepath.Join(dir, s.ID+".json")

	// Write to a unique temp file (not a fixed name) so concurrent Save calls
	// for the same session — e.g. the runner completing while the TUI cancels —
	// cannot clobber each other's temp file before the atomic rename.
	tmp, err := os.CreateTemp(dir, s.ID+"-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create session tmp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write session tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close session tmp: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename session: %w", err)
	}
	return nil
}

// Complete marks the session as completed.
func (s *Session) Complete(exitCode int, outputBytes int64, outputLines int) error {
	// If the user already killed this session via the TUI, the kill is the
	// final state — don't let the runner's natural completion overwrite it.
	if s.finalizedByKill() {
		return nil
	}
	now := time.Now()
	s.Status = "completed"
	s.ExitCode = &exitCode
	s.EndTime = &now
	s.Duration = now.Sub(s.StartTime).Round(time.Second).String()
	s.OutputBytes = outputBytes
	s.OutputLines = outputLines
	return s.Save()
}

// Fail marks the session as failed. It will not overwrite a session the user
// already killed via the TUI (that kill is the final state).
func (s *Session) Fail(exitCode int, errMsg string) error {
	if s.finalizedByKill() {
		return nil
	}
	now := time.Now()
	s.Status = "failed"
	s.ExitCode = &exitCode
	s.EndTime = &now
	s.Duration = now.Sub(s.StartTime).Round(time.Second).String()
	s.ErrorMsg = errMsg
	return s.Save()
}

// Kill marks the session as terminated by an explicit user action (e.g. the
// TUI). This is a final state: a later Complete/Fail from the runner detects it
// via finalizedByKill and leaves it untouched.
func (s *Session) Kill(exitCode int, reason string) error {
	now := time.Now()
	s.KilledByUser = true
	s.Status = "failed"
	s.ExitCode = &exitCode
	s.EndTime = &now
	s.Duration = now.Sub(s.StartTime).Round(time.Second).String()
	s.ErrorMsg = reason
	return s.Save()
}

// finalizedByKill reports whether the on-disk session was already marked failed
// by an explicit user/TUI kill, so other code paths must not overwrite it.
func (s *Session) finalizedByKill() bool {
	data, err := os.ReadFile(filepath.Join(config.SessionDirPath(), s.ID+".json"))
	if err != nil {
		return false
	}
	var disk Session
	if err := json.Unmarshal(data, &disk); err != nil {
		return false
	}
	return disk.KilledByUser
}

// LoadAll reads and returns all sessions, sorted newest first.
func LoadAll() []*Session {
	dir := config.SessionDirPath()
	// Glob "*.json" never matches the "*.json.tmp" temp files written by Save,
	// so no explicit filtering is needed here.
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		log.Warn().Err(err).Str("dir", dir).Msg("failed to glob session files")
		return nil
	}

	var sessions []*Session
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Warn().Err(err).Str("file", path).Msg("skipping unreadable session file")
			continue
		}
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			log.Warn().Err(err).Str("file", path).Msg("skipping corrupt session file")
			continue
		}
		sessions = append(sessions, &s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})

	return sessions
}

// OpenLog opens the session log file for writing.
func (s *Session) OpenLog() (*os.File, error) {
	return os.OpenFile(s.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
}
