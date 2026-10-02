package heed

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

type dockerCheck struct {
	name      string
	container string
	timeout   time.Duration
}

func NewDockerCheck(cfg CheckConfig) Checker {
	return &dockerCheck{
		name:      cfg.Name,
		container: cfg.Container,
		timeout:   parseDurationOrDefault(cfg.Timeout, 10*time.Second),
	}
}

func (c *dockerCheck) Name() string { return c.name }

func (c *dockerCheck) Check() Result {
	socketPath := "/var/run/docker.sock"
	if _, err := os.Stat(socketPath); err != nil {
		return Result{Name: c.name, State: StateUnknown, Value: "N/A", Message: "Docker not available"}
	}

	info, err := inspectContainer(socketPath, c.container, c.timeout)
	if err != nil {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "error",
			Message: fmt.Sprintf("Container %s: %v", c.container, err),
		}
	}

	if !info.State.Running {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   info.State.Status,
			Message: fmt.Sprintf("Container %s is %s", c.container, info.State.Status),
		}
	}

	state := StateOK
	msg := fmt.Sprintf("Container %s is running", c.container)

	if info.State.Health != nil && info.State.Health.Status != "" && info.State.Health.Status != "healthy" {
		state = StateWarning
		msg += fmt.Sprintf(" (health: %s)", info.State.Health.Status)
	}

	if info.RestartCount > 0 {
		msg += fmt.Sprintf(", restarts: %d", info.RestartCount)
	}

	return Result{
		Name:    c.name,
		State:   state,
		Value:   info.State.Status,
		Numeric: float64(info.RestartCount),
		Message: msg,
	}
}

type containerInfo struct {
	State struct {
		Status  string `json:"Status"`
		Running bool   `json:"Running"`
		Health  *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	RestartCount int `json:"RestartCount"`
}

func inspectContainer(socketPath, name string, timeout time.Duration) (containerInfo, error) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.DialTimeout("unix", socketPath, timeout)
			},
		},
		Timeout: timeout,
	}

	resp, err := client.Get(fmt.Sprintf("http://unix/containers/%s/json", name))
	if err != nil {
		return containerInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return containerInfo{}, fmt.Errorf("container %q not found", name)
	}
	if resp.StatusCode >= 300 {
		return containerInfo{}, fmt.Errorf("docker api: %s", resp.Status)
	}

	var info containerInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return containerInfo{}, err
	}

	return info, nil
}
