package heed

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type commandCheck struct {
	name    string
	command string
	timeout time.Duration
}

func NewCommandCheck(cfg CheckConfig) Checker {
	return &commandCheck{
		name:    cfg.Name,
		command: cfg.Command,
		timeout: parseDurationOrDefault(cfg.Timeout, 30*time.Second),
	}
}

func (c *commandCheck) Name() string { return c.name }

func (c *commandCheck) Check() Result {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", c.command)

	start := time.Now()
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	ms := elapsed.Milliseconds()

	if ctx.Err() == context.DeadlineExceeded {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "timeout",
			Numeric: float64(ms),
			Message: fmt.Sprintf("Command timed out after %s", c.timeout),
		}
	}

	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		exitCode := -1
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   fmt.Sprintf("exit %d", exitCode),
			Numeric: float64(ms),
			Message: fmt.Sprintf("Command failed: %s", truncate(msg, 200)),
		}
	}

	return Result{
		Name:    c.name,
		State:   StateOK,
		Value:   fmt.Sprintf("%dms", ms),
		Numeric: float64(ms),
		Message: fmt.Sprintf("Command OK in %dms", ms),
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
