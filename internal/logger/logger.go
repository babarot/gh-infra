package logger

import (
	"log/slog"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	charmlog "charm.land/log/v2"
)

const (
	// LevelTrace is the most verbose level, including API request/response bodies.
	LevelTrace = charmlog.DebugLevel - 1

	// EnvKey is the environment variable to set the log level.
	EnvKey = "GH_INFRA_LOG"
)

var (
	// Default is the package-level logger.
	Default *charmlog.Logger

	// Slog is the slog-compatible handler backed by charmbracelet/log.
	Slog *slog.Logger
)

func init() {
	Default = charmlog.NewWithOptions(os.Stderr, charmlog.Options{
		ReportTimestamp: true,
		Level:           charmlog.FatalLevel + 1, // silent by default
	})

	// Register TRACE level display name via styles
	styles := charmlog.DefaultStyles()
	styles.Levels[LevelTrace] = lipgloss.NewStyle().
		SetString("TRAC").
		Bold(true).
		MaxWidth(4).
		Foreground(lipgloss.Color("63"))
	Default.SetStyles(styles)

	Slog = slog.New(Default)
}

// Init configures the global logger based on the given level string.
func Init(level string) {
	lvl := parseLevel(level)
	Default.SetLevel(charmlog.Level(lvl))
	Slog = slog.New(Default)
}

func parseLevel(s string) charmlog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return LevelTrace
	case "debug":
		return charmlog.DebugLevel
	case "info":
		return charmlog.InfoLevel
	case "warn":
		return charmlog.WarnLevel
	case "error":
		return charmlog.ErrorLevel
	default:
		return charmlog.FatalLevel + 1 // silent
	}
}

// Enabled returns true if any log output is active (i.e. the level is not silent).
func Enabled() bool {
	return Default.GetLevel() <= charmlog.ErrorLevel
}

// IsTrace returns true if the current log level includes trace output.
func IsTrace() bool {
	return Default.GetLevel() <= LevelTrace
}

// IsDebug returns true if the current log level includes debug output.
func IsDebug() bool {
	return Default.GetLevel() <= charmlog.DebugLevel
}

// Convenience functions that delegate to the package-level logger.

func Trace(msg string, keyvals ...any) { Default.Log(LevelTrace, msg, keyvals...) }
func Debug(msg string, keyvals ...any) { Default.Debug(msg, keyvals...) }
func Info(msg string, keyvals ...any)  { Default.Info(msg, keyvals...) }
func Warn(msg string, keyvals ...any)  { Default.Warn(msg, keyvals...) }
func Error(msg string, keyvals ...any) { Default.Error(msg, keyvals...) }
