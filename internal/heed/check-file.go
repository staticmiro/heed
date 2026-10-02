package heed

import (
	"fmt"
	"os"
	"time"
)

type fileCheck struct {
	name   string
	path   string
	maxAge time.Duration
}

func NewFileCheck(cfg CheckConfig) Checker {
	var maxAge time.Duration
	if cfg.MaxAge != "" {
		maxAge, _ = time.ParseDuration(cfg.MaxAge)
	}
	return &fileCheck{name: cfg.Name, path: cfg.Path, maxAge: maxAge}
}

func (c *fileCheck) Name() string { return c.name }

func (c *fileCheck) Check() Result {
	info, err := os.Stat(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Name: c.name, State: StateCritical, Value: "missing", Message: fmt.Sprintf("%s does not exist", c.path)}
		}
		return Result{Name: c.name, State: StateUnknown, Value: "error", Message: fmt.Sprintf("%s: %v", c.path, err)}
	}

	if c.maxAge > 0 {
		age := time.Since(info.ModTime())
		if age > c.maxAge {
			return Result{
				Name:    c.name,
				State:   StateCritical,
				Value:   formatAge(age),
				Numeric: age.Hours(),
				Message: fmt.Sprintf("%s is %s old (max: %s)", c.path, formatAge(age), formatAge(c.maxAge)),
			}
		}
		return Result{
			Name:    c.name,
			State:   StateOK,
			Value:   formatAge(age),
			Numeric: age.Hours(),
			Message: fmt.Sprintf("%s is %s old", c.path, formatAge(age)),
		}
	}

	return Result{
		Name:    c.name,
		State:   StateOK,
		Value:   formatBytes(info.Size()),
		Numeric: float64(info.Size()),
		Message: fmt.Sprintf("%s exists, %s", c.path, formatBytes(info.Size())),
	}
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1fGB", float64(b)/(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
}
