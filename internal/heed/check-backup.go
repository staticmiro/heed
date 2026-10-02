package heed

import (
	"fmt"
	"os"
	"time"
)

type backupCheck struct {
	name   string
	path   string
	maxAge time.Duration
}

func NewBackupCheck(cfg CheckConfig) Checker {
	var maxAge time.Duration
	if cfg.MaxAge != "" {
		maxAge, _ = time.ParseDuration(cfg.MaxAge)
	}
	return &backupCheck{name: cfg.Name, path: cfg.Path, maxAge: maxAge}
}

func (c *backupCheck) Name() string { return c.name }

func (c *backupCheck) Check() Result {
	info, err := os.Stat(c.path)
	if err != nil {
		return Result{Name: c.name, State: StateCritical, Value: "missing", Message: fmt.Sprintf("%s does not exist", c.path)}
	}

	if !info.IsDir() {
		return Result{Name: c.name, State: StateUnknown, Value: "error", Message: fmt.Sprintf("%s is not a directory", c.path)}
	}

	entries, err := os.ReadDir(c.path)
	if err != nil {
		return Result{Name: c.name, State: StateUnknown, Value: "error", Message: err.Error()}
	}

	if len(entries) == 0 {
		return Result{Name: c.name, State: StateCritical, Value: "empty", Message: fmt.Sprintf("no backups in %s", c.path)}
	}

	var newest time.Time
	var newestName string
	var newestSize int64

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err == nil {
			if newest.IsZero() || info.ModTime().After(newest) {
				newest = info.ModTime()
				newestName = e.Name()
				newestSize = info.Size()
			}
		}
	}

	if newest.IsZero() {
		return Result{Name: c.name, State: StateCritical, Value: "empty", Message: fmt.Sprintf("no files in %s", c.path)}
	}

	age := time.Since(newest)

	if c.maxAge > 0 && age > c.maxAge {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   formatAge(age),
			Numeric: age.Hours(),
			Message: fmt.Sprintf("newest backup %s is %s old (max: %s)", newestName, formatAge(age), formatAge(c.maxAge)),
		}
	}

	return Result{
		Name:    c.name,
		State:   StateOK,
		Value:   formatAge(age),
		Numeric: age.Hours(),
		Message: fmt.Sprintf("newest backup %s is %s old, size %s", newestName, formatAge(age), formatBytes(newestSize)),
	}
}
