package heed

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

type processCheck struct {
	name      string
	match     string
	minCount  int
	maxCPU    int
	maxMemory int64
}

func parseMemoryLimit(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return 0
	}
	var mult int64 = 1
	if strings.HasSuffix(s, "GB") {
		mult = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	} else if strings.HasSuffix(s, "MB") {
		mult = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	} else if strings.HasSuffix(s, "KB") {
		mult = 1024
		s = strings.TrimSuffix(s, "KB")
	} else if strings.HasSuffix(s, "B") {
		s = strings.TrimSuffix(s, "B")
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return int64(val * float64(mult))
}

func NewProcessCheck(cfg CheckConfig) Checker {
	minCount := cfg.MinCount
	if minCount == 0 {
		minCount = 1
	}
	return &processCheck{
		name:      cfg.Name,
		match:     cfg.Match,
		minCount:  minCount,
		maxCPU:    cfg.MaxCPU,
		maxMemory: parseMemoryLimit(cfg.MaxMemory),
	}
}

func (c *processCheck) Name() string { return c.name }

func (c *processCheck) Check() Result {
	procs, err := process.Processes()
	if err != nil {
		return Result{Name: c.name, State: StateUnknown, Value: "error", Message: fmt.Sprintf("process list: %v", err)}
	}

	count := 0
	var totalCPU float64
	var totalMem int64

	for _, p := range procs {
		name, _ := p.Name()
		cmdline, _ := p.Cmdline()
		if strings.Contains(strings.ToLower(name), strings.ToLower(c.match)) ||
			strings.Contains(strings.ToLower(cmdline), strings.ToLower(c.match)) {
			count++
			if c.maxCPU > 0 {
				if perc, err := p.CPUPercent(); err == nil {
					totalCPU += perc
				}
			}
			if c.maxMemory > 0 {
				if mem, err := p.MemoryInfo(); err == nil {
					totalMem += int64(mem.RSS)
				}
			}
		}
	}

	if count < c.minCount {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   fmt.Sprintf("%d", count),
			Numeric: float64(count),
			Message: fmt.Sprintf("Process %s: %d found, expected >= %d", c.match, count, c.minCount),
		}
	}

	if c.maxCPU > 0 && int(totalCPU) > c.maxCPU {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   fmt.Sprintf("%.1f%%", totalCPU),
			Numeric: totalCPU,
			Message: fmt.Sprintf("Process %s CPU %.1f%% > max %d%%", c.match, totalCPU, c.maxCPU),
		}
	}

	if c.maxMemory > 0 && totalMem > c.maxMemory {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   formatBytes(totalMem),
			Numeric: float64(totalMem),
			Message: fmt.Sprintf("Process %s Mem %s > max %s", c.match, formatBytes(totalMem), formatBytes(c.maxMemory)),
		}
	}

	return Result{
		Name:    c.name,
		State:   StateOK,
		Value:   fmt.Sprintf("%d running", count),
		Numeric: float64(count),
		Message: fmt.Sprintf("Process %s: %d running (CPU %.1f%%, Mem %s)", c.match, count, totalCPU, formatBytes(totalMem)),
	}
}

type systemdCheck struct {
	name          string
	service       string
	expectedState string
}

func NewSystemdCheck(cfg CheckConfig) Checker {
	state := cfg.State
	if state == "" {
		state = "active"
	}
	return &systemdCheck{name: cfg.Name, service: cfg.Match, expectedState: state}
}

func (c *systemdCheck) Name() string { return c.name }

func (c *systemdCheck) Check() Result {
	cmdName := "is-active"
	if c.expectedState == "enabled" || c.expectedState == "disabled" {
		cmdName = "is-enabled"
	}

	out, err := exec.Command("systemctl", cmdName, c.service).Output()
	status := strings.TrimSpace(string(out))
	if status == "" {
		if err != nil {
			status = "unknown/error"
		} else {
			status = "unknown"
		}
	}

	// is-active returns non-zero for anything other than active
	// is-enabled returns non-zero for anything other than enabled
	if status != c.expectedState {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   status,
			Message: fmt.Sprintf("Service %s is %s (expected %s)", c.service, status, c.expectedState),
		}
	}

	return Result{
		Name:    c.name,
		State:   StateOK,
		Value:   status,
		Message: fmt.Sprintf("Service %s is %s", c.service, status),
	}
}
