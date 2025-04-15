package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/rs/zerolog"
)

// MessageType defines the type of message
type MessageType int

const (
	TypeSuccess MessageType = iota // GREEN
	TypeInfo                       // BLUE
	TypeNote                       // GREY
	TypeWarning                    // ORANGE
	TypeError                      // RED
)

// Logger provides structured, colorized logging for Styx
type Logger struct {
	zlog        zerolog.Logger
	mu          sync.Mutex
	progressBar *ProgressBar
	colors      map[MessageType]*color.Color
	isVerbose   bool
	output      io.Writer
}

// ProgressBar represents a simple progress indicator
type ProgressBar struct {
	total    int
	current  int
	message  string
	spinChar int
	lastLine string
	isActive bool
}

// New creates a new logger
func New(verbose bool) *Logger {
	output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}

	zlog := zerolog.New(output).With().Timestamp().Logger()
	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	return &Logger{
		zlog:      zlog,
		output:    os.Stderr,
		isVerbose: verbose,
		colors: map[MessageType]*color.Color{
			TypeSuccess: color.New(color.FgGreen, color.Bold),
			TypeInfo:    color.New(color.FgBlue),
			TypeNote:    color.New(color.FgWhite),
			TypeWarning: color.New(color.FgYellow),
			TypeError:   color.New(color.FgRed, color.Bold),
		},
	}
}

// formatPrefix returns a colored prefix based on message type
func (l *Logger) formatPrefix(msgType MessageType) string {
	var prefix string

	switch msgType {
	case TypeSuccess:
		prefix = "[SUCCESS]"
	case TypeInfo:
		prefix = "[INFO]"
	case TypeNote:
		prefix = "[NOTE]"
	case TypeWarning:
		prefix = "[WARNING]"
	case TypeError:
		prefix = "[ERROR]"
	}

	return l.colors[msgType].Sprint(prefix)
}

// Log logs a message of the specified type
func (l *Logger) Log(msgType MessageType, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// clear progress bar if active
	if l.progressBar != nil && l.progressBar.isActive {
		_, err := fmt.Fprintln(l.output, "\r"+strings.Repeat(" ", len(l.progressBar.lastLine))+"\r")
		if err != nil {
			return
		}
	}

	message := fmt.Sprintf(format, args...)
	_, err := fmt.Fprintf(l.output, "%s %s\n", l.formatPrefix(msgType), message)
	if err != nil {
		return
	}

	// redraw if otherwise
	if l.progressBar != nil && l.progressBar.isActive {
		l.drawProgressBar()
	}
}

// Success logs a success message (green)
func (l *Logger) Success(format string, args ...interface{}) {
	l.Log(TypeSuccess, format, args...)
}

// Info logs an information message (blue)
func (l *Logger) Info(format string, args ...interface{}) {
	l.Log(TypeInfo, format, args...)
}

// Note logs a note message (grey)
func (l *Logger) Note(format string, args ...interface{}) {
	l.Log(TypeNote, format, args...)
}

// Warning logs a warning message (orange/yellow)
func (l *Logger) Warning(format string, args ...interface{}) {
	l.Log(TypeWarning, format, args...)
}

// Error logs an error message (red)
func (l *Logger) Error(format string, args ...interface{}) {
	l.Log(TypeError, format, args...)
}

// StartProgress starts a new progress indicator
func (l *Logger) StartProgress(total int, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.progressBar = &ProgressBar{
		total:    total,
		current:  0,
		message:  message,
		spinChar: 0,
		isActive: true,
	}

	l.drawProgressBar()
}

// UpdateProgress updates the progress indicator
func (l *Logger) UpdateProgress(current int, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.progressBar == nil {
		return
	}

	l.progressBar.current = current
	if message != "" {
		l.progressBar.message = message
	}

	l.drawProgressBar()
}

// StopProgress stops the progress indicator
func (l *Logger) StopProgress() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.progressBar == nil {
		return
	}

	// clear
	_, err := fmt.Fprint(l.output, "\r"+strings.Repeat(" ", len(l.progressBar.lastLine))+"\r")
	if err != nil {
		return
	}
	l.progressBar.isActive = false
}

// drawProgressBar draws the progress bar
func (l *Logger) drawProgressBar() {
	if l.progressBar == nil {
		return
	}

	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinChar := spinner[l.progressBar.spinChar]
	l.progressBar.spinChar = (l.progressBar.spinChar + 1) % len(spinner)

	percentage := 0
	if l.progressBar.total > 0 {
		percentage = (l.progressBar.current * 100) / l.progressBar.total
	}

	progressText := fmt.Sprintf("%s %s [%d%%] %s",
		l.colors[TypeInfo].Sprint("[PROGRESS]"),
		spinChar,
		percentage,
		l.progressBar.message)

	l.progressBar.lastLine = progressText
	_, err := fmt.Fprintln(l.output, "\r"+progressText)
	if err != nil {
		return
	}
}

// BuilderEvent represents an event during the build process
type BuilderEvent struct {
	Type        MessageType
	Message     string
	Source      string
	Line        int
	Column      int
	Code        string
	Suggestions []string
}

func (l *Logger) ReportBuildEvent(event BuilderEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.progressBar != nil && l.progressBar.isActive {
		_, err := fmt.Fprint(l.output, "\r"+strings.Repeat(" ", len(l.progressBar.lastLine))+"\r")
		if err != nil {
			return
		}
	}

	prefix := l.formatPrefix(event.Type)
	var location string
	if event.Source != "" {
		if event.Line > 0 {
			if event.Column > 0 {
				location = fmt.Sprintf("%s:%d:%d:", event.Source, event.Line, event.Column)
			} else {
				location = fmt.Sprintf("%s:%d:", event.Source, event.Line)
			}
		} else {
			location = fmt.Sprintf("%s:", event.Source)
		}
	}

	if location != "" {
		_, err := color.New(color.FgCyan).Fprintf(l.output, "%s ", location)
		if err != nil {
			return
		}
	}

	_, err := fmt.Fprintf(l.output, "%s %s\n", prefix, event.Message)
	if err != nil {
		return
	}
	if event.Code != "" && l.isVerbose {
		_, err := fmt.Fprintf(l.output, "    %s\n", event.Code)
		if err != nil {
			return
		}
	}

	for _, suggestion := range event.Suggestions {
		_, err := fmt.Fprintf(l.output, "    %s\n", suggestion)
		if err != nil {
			return
		}
	}

	if l.progressBar != nil && l.progressBar.isActive {
		l.drawProgressBar()
	}
}
