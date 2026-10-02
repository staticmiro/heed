package heed

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Timestamp time.Time `json:"ts"`
	Check     string    `json:"check"`
	PrevState State     `json:"prev"`
	State     State     `json:"state"`
	Value     string    `json:"value"`
	Numeric   float64   `json:"num"`
	Message   string    `json:"msg"`
}

type EventLog struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	maxSize int64
	writes  int
}

func NewEventLog(path string, maxSizeStr string) (*EventLog, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	return &EventLog{
		file:    f,
		path:    path,
		maxSize: parseSize(maxSizeStr),
	}, nil
}

func (l *EventLog) Write(event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = l.file.Write(append(data, '\n'))
	if err != nil {
		return err
	}

	l.writes++
	if l.writes%100 == 0 {
		l.maybeRotate()
	}

	return nil
}

func (l *EventLog) maybeRotate() {
	info, err := l.file.Stat()
	if err != nil || info.Size() < l.maxSize {
		return
	}

	l.file.Close()
	os.Remove(l.path + ".1")
	os.Rename(l.path, l.path+".1")
	l.file, _ = os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}

func (l *EventLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

func ReadEvents(path string, limit int) ([]Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}

	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}

	events := make([]Event, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		events = append(events, e)
	}

	return events, nil
}

func parseSize(s string) int64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	multiplier := int64(1)

	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		s = s[:len(s)-2]
	}

	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 50 * 1024 * 1024
	}
	return n * multiplier
}
