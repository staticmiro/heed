package heed

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type logCheck struct {
	name       string
	path       string
	pattern    string
	ignoreCase bool
	regex      *regexp.Regexp

	offset      int64
	inode       uint64
	initialized bool
}

func NewLogCheck(cfg CheckConfig) Checker {
	var re *regexp.Regexp

	pattern := cfg.Pattern
	if cfg.IgnoreCase {
		pattern = "(?i)" + pattern
	}

	re, _ = regexp.Compile(pattern)

	return &logCheck{
		name:       cfg.Name,
		path:       cfg.Path,
		pattern:    cfg.Pattern,
		ignoreCase: cfg.IgnoreCase,
		regex:      re,
	}
}

func (c *logCheck) Name() string { return c.name }

func (c *logCheck) Check() Result {
	info, err := os.Stat(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Name: c.name, State: StateUnknown, Value: "missing", Message: "Log file does not exist"}
		}
		return Result{Name: c.name, State: StateUnknown, Value: "error", Message: err.Error()}
	}

	// For simplicity, we use size to detect rotation/truncation
	// If size is smaller than our offset, it was truncated
	if c.initialized && info.Size() < c.offset {
		c.offset = 0 // file was truncated
	}

	file, err := os.Open(c.path)
	if err != nil {
		return Result{Name: c.name, State: StateUnknown, Value: "error", Message: err.Error()}
	}
	defer file.Close()

	if !c.initialized {
		c.initialized = true
		c.offset = info.Size() // start at end
		return Result{Name: c.name, State: StateOK, Value: "initialized", Message: "Log check initialized"}
	}

	if c.offset == info.Size() {
		return Result{Name: c.name, State: StateOK, Value: "no new logs", Message: "No new lines"}
	}

	_, err = file.Seek(c.offset, io.SeekStart)
	if err != nil {
		c.offset = info.Size()
		return Result{Name: c.name, State: StateUnknown, Value: "seek error", Message: err.Error()}
	}

	scanner := bufio.NewScanner(file)
	matchedLines := []string{}

	for scanner.Scan() {
		line := scanner.Text()
		if c.regex != nil {
			if c.regex.MatchString(line) {
				matchedLines = append(matchedLines, line)
			}
		} else {
			if c.ignoreCase {
				if strings.Contains(strings.ToLower(line), strings.ToLower(c.pattern)) {
					matchedLines = append(matchedLines, line)
				}
			} else {
				if strings.Contains(line, c.pattern) {
					matchedLines = append(matchedLines, line)
				}
			}
		}
	}

	// Update offset
	if currentOffset, err := file.Seek(0, io.SeekCurrent); err == nil {
		c.offset = currentOffset
	}

	if len(matchedLines) > 0 {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   fmt.Sprintf("%d matches", len(matchedLines)),
			Numeric: float64(len(matchedLines)),
			Message: fmt.Sprintf("Found %d matches:\n- %s", len(matchedLines), truncate(matchedLines[0], 200)),
		}
	}

	return Result{
		Name:    c.name,
		State:   StateOK,
		Value:   "clean",
		Message: "No pattern matches in new logs",
	}
}
