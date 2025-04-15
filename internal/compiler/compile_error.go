package compiler

import (
	"bufio"
	"regexp"
	"strconv"
	"strings"

	"github.com/deviceix/styx/internal/logger"
)

// ErrorParser parses compiler error messages
type ErrorParser struct {
	logger *logger.Logger
}

// NewErrorParser creates a new error parser
func NewErrorParser(log *logger.Logger) *ErrorParser {
	return &ErrorParser{
		logger: log,
	}
}

// ParseGCCOutput parses GCC/Clang error output
func (p *ErrorParser) ParseGCCOutput(output, sourceFile string) []logger.BuilderEvent {
	var events []logger.BuilderEvent
	// defo need better message matching lol
	reFileLocation := regexp.MustCompile(`^(.*?):(\d+):(?:(\d+):)?\s+(warning|error|note|fatal error):\s+(.*)$`)
	scanner := bufio.NewScanner(strings.NewReader(output))
	var currentEvent *logger.BuilderEvent

	for scanner.Scan() {
		line := scanner.Text()
		if matches := reFileLocation.FindStringSubmatch(line); matches != nil {
			file := matches[1]
			lineNum, _ := strconv.Atoi(matches[2])
			colNum := 0
			if matches[3] != "" {
				colNum, _ = strconv.Atoi(matches[3])
			}

			errorType := matches[4]
			message := matches[5]

			var msgType logger.MessageType
			switch errorType {
			case "warning":
				msgType = logger.TypeWarning
			case "error", "fatal error":
				msgType = logger.TypeError
			case "note":
				msgType = logger.TypeNote
			default:
				msgType = logger.TypeInfo
			}

			// If this is a note for an existing error, add it to the suggestions
			if errorType == "note" && currentEvent != nil {
				currentEvent.Suggestions = append(currentEvent.Suggestions, message)
				continue
			}

			// Create a new event
			event := logger.BuilderEvent{
				Type:        msgType,
				Message:     message,
				Source:      file,
				Line:        lineNum,
				Column:      colNum,
				Suggestions: []string{},
			}

			events = append(events, event)
			currentEvent = &events[len(events)-1]
		} else if strings.TrimSpace(line) != "" && currentEvent != nil {
			if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
				currentEvent.Code = strings.TrimSpace(line)
			} else {
				currentEvent.Suggestions = append(currentEvent.Suggestions, line)
			}
		}
	}

	return events
}

// Report formats and logs the parsed errors
func (p *ErrorParser) Report(output, sourceFile string) {
	events := p.ParseGCCOutput(output, sourceFile)

	for _, event := range events {
		p.logger.ReportBuildEvent(event)
	}
}
