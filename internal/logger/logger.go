package logger

import (
	"encoding/json"
	"log"
	"os"
	"regexp"
	"time"
)

type LogLevel string

const (
	InfoLevel  LogLevel = "INFO"
	ErrorLevel LogLevel = "ERROR"
	DebugLevel LogLevel = "DEBUG"
)

// LogEntry describes the structure of a log message
type LogEntry struct {
	Time    string   `json:"time"`
	Level   LogLevel `json:"level"`
	Module  string   `json:"module,omitempty"`
	Message string   `json:"message"`
	Error   string   `json:"error,omitempty"`
}

// Logger is a centralized structured logger
type Logger struct {
	out *log.Logger
}

// New creates a new Logger
func New() *Logger {
	return &Logger{
		out: log.New(os.Stdout, "", 0),
	}
}

// Anonymize replaces sensitive information in logs (emails, tokens, IDs)
func Anonymize(s string) string {
	// Replace emails with [REDACTED_EMAIL]
	emailRegex := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	s = emailRegex.ReplaceAllString(s, "[REDACTED_EMAIL]")

	// Replace JWT tokens (simple pattern) with [REDACTED_TOKEN]
	tokenRegex := regexp.MustCompile(`eyJ[^\s]+`)
	s = tokenRegex.ReplaceAllString(s, "[REDACTED_TOKEN]")

	// Replace user IDs with [USER_ID]
	userIDRegex := regexp.MustCompile(`\buser_id\s*=\s*\d+\b`)
	s = userIDRegex.ReplaceAllString(s, "user_id=[USER_ID]")

	return s
}

// internal log function
func (l *Logger) log(module string, level LogLevel, msg string, err error) {
	entry := LogEntry{
		Time:    time.Now().Format(time.RFC3339),
		Level:   level,
		Module:  module,
		Message: Anonymize(msg),
	}
	if err != nil {
		entry.Error = Anonymize(err.Error())
	}
	data, _ := json.Marshal(entry)
	l.out.Println(string(data))
}

// --- Convenient methods ---
func (l *Logger) Info(module, msg string) {
	l.log(module, InfoLevel, msg, nil)
}

func (l *Logger) Debug(module, msg string) {
	l.log(module, DebugLevel, msg, nil)
}

func (l *Logger) Error(module, msg string, err error) {
	l.log(module, ErrorLevel, msg, err)
}
