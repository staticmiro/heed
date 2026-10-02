package heed

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type updatesCheck struct {
	name    string
	manager string
	warning int
}

func NewUpdatesCheck(cfg CheckConfig) Checker {
	manager := strings.ToLower(cfg.PackageManager)
	if manager == "" {
		manager = "apt"
	}
	return &updatesCheck{name: cfg.Name, manager: manager, warning: cfg.Warning}
}

func (c *updatesCheck) Name() string { return c.name }

func (c *updatesCheck) Check() Result {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var count int
	var msg string

	if c.manager == "apt" {
		// Try apt-check first (fast on Ubuntu/Debian)
		if _, err := os.Stat("/usr/lib/update-notifier/apt-check"); err == nil {
			out, err := exec.CommandContext(ctx, "/usr/lib/update-notifier/apt-check", "--stderr").CombinedOutput()
			if err == nil {
				// output format: "updates;security_updates" e.g. "5;0"
				parts := strings.Split(strings.TrimSpace(string(out)), ";")
				if len(parts) >= 1 {
					count, _ = strconv.Atoi(parts[0])
					msg = fmt.Sprintf("%d packages can be updated", count)
				}
			}
		}

		// Fallback to apt list if apt-check didn't work and count is still 0
		if msg == "" {
			out, err := exec.CommandContext(ctx, "apt", "list", "--upgradable").Output()
			if err != nil {
				return Result{Name: c.name, State: StateUnknown, Value: "error", Message: fmt.Sprintf("apt list error: %v", err)}
			}
			lines := strings.Split(string(out), "\n")
			// First line is "Listing...", the rest are packages
			for _, line := range lines {
				if strings.Contains(line, "upgradable") {
					count++
				}
			}
			msg = fmt.Sprintf("%d packages can be updated", count)
		}
	} else {
		return Result{Name: c.name, State: StateUnknown, Value: "error", Message: fmt.Sprintf("unsupported package manager: %s", c.manager)}
	}

	state := StateOK
	if c.warning > 0 && count >= c.warning {
		state = StateWarning
	}

	return Result{
		Name:    c.name,
		State:   state,
		Value:   fmt.Sprintf("%d updates", count),
		Numeric: float64(count),
		Message: msg,
	}
}
