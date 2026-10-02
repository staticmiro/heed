package heed

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type fail2banCheck struct {
	name    string
	jail    string
	warning int
}

func NewFail2banCheck(cfg CheckConfig) Checker {
	jail := cfg.Jail
	if jail == "" {
		jail = "sshd"
	}
	return &fail2banCheck{name: cfg.Name, jail: jail, warning: cfg.Warning}
}

func (c *fail2banCheck) Name() string { return c.name }

func (c *fail2banCheck) Check() Result {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "fail2ban-client", "status", c.jail).CombinedOutput()
	if err != nil {
		return Result{Name: c.name, State: StateUnknown, Value: "error", Message: fmt.Sprintf("fail2ban-client error: %v", err)}
	}

	lines := strings.Split(string(out), "\n")
	var banned int
	found := false

	for _, line := range lines {
		if strings.Contains(line, "Currently banned:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				if n, err := strconv.Atoi(val); err == nil {
					banned = n
					found = true
				}
			}
		}
	}

	if !found {
		return Result{
			Name:    c.name,
			State:   StateUnknown,
			Value:   "parse error",
			Message: fmt.Sprintf("could not parse fail2ban output for jail %s", c.jail),
		}
	}

	state := StateOK
	if c.warning > 0 && banned >= c.warning {
		state = StateWarning
	}

	return Result{
		Name:    c.name,
		State:   state,
		Value:   fmt.Sprintf("%d banned", banned),
		Numeric: float64(banned),
		Message: fmt.Sprintf("Jail %s: %d IP(s) currently banned", c.jail, banned),
	}
}
