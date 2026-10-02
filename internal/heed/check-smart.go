package heed

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type smartCheck struct {
	name    string
	device  string
	timeout time.Duration
}

func NewSmartCheck(cfg CheckConfig) Checker {
	device := cfg.Device
	if device == "" {
		device = "/dev/sda"
	}
	return &smartCheck{
		name:    cfg.Name,
		device:  device,
		timeout: parseDurationOrDefault(cfg.Timeout, 5*time.Second),
	}
}

func (c *smartCheck) Name() string { return c.name }

func (c *smartCheck) Check() Result {
	cmd := exec.Command("smartctl", "-H", "-j", c.device)
	output, err := cmd.Output()

	// smartctl might return non-zero exit code even if SMART says passed (e.g. if errors are logged)
	// so we shouldn't fail simply on err != nil, we must parse the JSON.

	if len(output) == 0 {
		return Result{
			Name:    c.name,
			State:   StateUnknown,
			Value:   "error",
			Message: fmt.Sprintf("smartctl failed: %v", err),
		}
	}

	var data struct {
		SmartStatus struct {
			Passed bool `json:"passed"`
		} `json:"smart_status"`
	}

	if err := json.Unmarshal(output, &data); err != nil {
		return Result{
			Name:    c.name,
			State:   StateUnknown,
			Value:   "error",
			Message: fmt.Sprintf("smartctl json parse error: %v", err),
		}
	}

	if !data.SmartStatus.Passed {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "FAIL",
			Numeric: 0,
			Message: fmt.Sprintf("SMART status check failed for %s", c.device),
		}
	}

	return Result{
		Name:    c.name,
		State:   StateOK,
		Value:   "PASSED",
		Numeric: 1,
		Message: fmt.Sprintf("SMART status passed for %s", c.device),
	}
}
