package heed

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

type CheckStatus struct {
	Name    string
	State   State
	Value   string
	Message string
}

type Monitor struct {
	mu        sync.RWMutex
	config    Config
	checks    []Checker
	notifiers []Notifier
	eventLog  *EventLog
	states    map[string]State
	cooldowns map[string]time.Time
	trends    map[string]*TrendBuffer
	logger    *log.Logger
	startTime time.Time
	deps      map[string][]string
	silence   *SilenceManager
}

func NewMonitor(cfg Config, logger *log.Logger) (*Monitor, error) {
	setupHostEnv()

	eventLog, err := NewEventLog(cfg.Monitor.DataFile, cfg.Monitor.MaxDataSize)
	if err != nil {
		return nil, fmt.Errorf("event log: %w", err)
	}

	deps := make(map[string][]string)
	for _, c := range cfg.Check {
		if len(c.DependsOn) > 0 {
			deps[c.Name] = c.DependsOn
		}
	}

	return &Monitor{
		config:    cfg,
		checks:    BuildChecks(cfg),
		notifiers: BuildNotifiers(cfg),
		eventLog:  eventLog,
		states:    make(map[string]State),
		cooldowns: make(map[string]time.Time),
		trends:    make(map[string]*TrendBuffer),
		logger:    logger,
		startTime: time.Now(),
		deps:      deps,
		silence:   NewSilenceManager(cfg.Monitor.DataFile),
	}, nil
}

func (m *Monitor) ReloadConfig(path string) error {
	cfg, err := LoadConfig(path)
	if err != nil {
		return err
	}
	if errs := ValidateConfig(cfg); len(errs) > 0 {
		return fmt.Errorf("config validation failed")
	}

	deps := make(map[string][]string)
	for _, c := range cfg.Check {
		if len(c.DependsOn) > 0 {
			deps[c.Name] = c.DependsOn
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = cfg
	m.checks = BuildChecks(cfg)
	m.notifiers = BuildNotifiers(cfg)
	m.deps = deps
	m.logger.Printf("config reloaded successfully from %s", path)
	return nil
}

func (m *Monitor) Run(ctx context.Context) {
	if d := m.config.Monitor.StartupGraceDuration(); d > 0 {
		m.logger.Printf("startup grace period for %s", d)
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return
		}
	}

	interval := m.config.Monitor.IntervalDuration()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	m.logger.Printf("monitoring %s with %d checks, interval %s", m.config.Server.Name, len(m.checks), interval)

	m.runCycle()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.runCycle()
		}
	}
}

func (m *Monitor) RunOnce() []Result {
	setupHostEnv()
	m.mu.RLock()
	defer m.mu.RUnlock()
	var results []Result
	for _, c := range m.checks {
		results = append(results, c.Check())
	}
	return results
}

func (m *Monitor) Status() []CheckStatus {
	results := m.RunOnce()
	statuses := make([]CheckStatus, len(results))
	for i, r := range results {
		statuses[i] = CheckStatus{
			Name:    r.Name,
			State:   r.State,
			Value:   r.Value,
			Message: r.Message,
		}
	}
	return statuses
}

func (m *Monitor) Close() error {
	return m.eventLog.Close()
}

func (m *Monitor) runCycle() {
	m.mu.RLock()
	cooldown := m.config.Alerts.CooldownDuration()
	checks := m.checks
	notifiers := m.notifiers
	deps := m.deps
	serverName := m.config.Server.Name
	notifyRecovery := m.config.Alerts.NotifyRecovery
	interval := m.config.Monitor.IntervalDuration()
	m.mu.RUnlock()

	results := make(map[string]Result)
	for _, c := range checks {
		results[c.Name()] = c.Check()
	}

	failedParents := make(map[string][]string) // parentName -> []dependentNames
	for name, result := range results {
		if result.State != StateOK {
			for _, parent := range deps[name] {
				if pRes, ok := results[parent]; ok && pRes.State != StateOK {
					failedParents[parent] = append(failedParents[parent], name)
				}
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, c := range checks {
		result := results[c.Name()]
		prev, exists := m.states[result.Name]
		if !exists {
			prev = StateUnknown
		}

		if result.State == prev {
			if result.Numeric > 0 && (result.State == StateOK || result.State == StateWarning) {
				tb, ok := m.trends[result.Name]
				if !ok {
					tb = NewTrendBuffer(20)
					m.trends[result.Name] = tb
				}
				tb.Add(result.Numeric)

				threshold := result.Threshold

				if threshold > 0 && tb.IsIncreasing() {
					eta := tb.EstimateTimeToThreshold(threshold, interval)
					if eta > 0 && eta < 1*time.Hour {
						m.logger.Printf("TREND ALERT: %s is increasing, ETA to critical: %s", result.Name, eta)
						event := Event{
							Timestamp: time.Now().UTC(),
							Check:     result.Name + "_trend",
							PrevState: StateOK,
							State:     StateWarning,
							Value:     fmt.Sprintf("ETA %s", eta),
							Numeric:   eta.Hours(),
							Message:   FormatTrendAlert(result.Name, threshold, eta),
						}

						cooldownKey := event.Check + ":" + string(event.State)
						if last, ok := m.cooldowns[cooldownKey]; !ok || time.Since(last) >= cooldown {
							m.cooldowns[cooldownKey] = time.Now()
							for _, n := range notifiers {
								n.Send(event, serverName)
							}
						}
					}
				}
			}
			continue
		}

		m.states[result.Name] = result.State

		event := Event{
			Timestamp: time.Now().UTC(),
			Check:     result.Name,
			PrevState: prev,
			State:     result.State,
			Value:     result.Value,
			Numeric:   result.Numeric,
			Message:   result.Message,
		}

		if err := m.eventLog.Write(event); err != nil {
			m.logger.Printf("event log error: %v", err)
		}

		if result.State == StateOK && !notifyRecovery {
			continue
		}

		suppressedByParent := false
		if result.State != StateOK {
			for _, parent := range deps[result.Name] {
				if pRes, ok := results[parent]; ok && pRes.State != StateOK {
					m.logger.Printf("suppressing alert for %s because parent %s is failing", result.Name, parent)
					suppressedByParent = true
					break
				}
			}
		}
		if suppressedByParent {
			continue
		}

		if deps, ok := failedParents[result.Name]; ok && result.State != StateOK {
			event.Message += fmt.Sprintf("\n\n%d dependent check(s) also failing:\n", len(deps))
			for _, d := range deps {
				event.Message += fmt.Sprintf("- %s\n", d)
			}
			event.Message += fmt.Sprintf("\nLikely root cause: %s", result.Name)
		}

		cooldownKey := result.Name + ":" + string(result.State)
		if last, ok := m.cooldowns[cooldownKey]; ok && time.Since(last) < cooldown {
			continue
		}

		if m.silence.IsSilenced(result.Name) {
			m.logger.Printf("suppressing alert for %s due to silence rule", result.Name)
			continue
		}

		m.cooldowns[cooldownKey] = time.Now()

		for _, n := range notifiers {
			if err := n.Send(event, serverName); err != nil {
				m.logger.Printf("notify %s error: %v", n.Name(), err)
			}
		}
	}
}

func BuildChecks(cfg Config) []Checker {
	var checks []Checker

	if cfg.CPU.Enabled {
		checks = append(checks, NewCPUCheck(cfg.CPU.Warning, cfg.CPU.Critical))
	}
	if cfg.Memory.Enabled {
		checks = append(checks, NewMemoryCheck(cfg.Memory.Warning, cfg.Memory.Critical))
	}
	if cfg.Swap.Enabled {
		checks = append(checks, NewSwapCheck(cfg.Swap.Warning, cfg.Swap.Critical))
	}
	if cfg.Disk.Enabled {
		checks = append(checks, NewDiskCheck(cfg.Disk.Path, cfg.Disk.Warning, cfg.Disk.Critical))
	}
	if cfg.Temperature.Enabled {
		checks = append(checks, NewTemperatureCheck(cfg.Temperature.Warning, cfg.Temperature.Critical))
	}

	checks = append(checks, NewLoadCheck())
	checks = append(checks, NewUptimeCheck())

	for _, cc := range cfg.Check {
		switch cc.Type {
		case "http":
			checks = append(checks, NewHTTPCheck(cc))
		case "tcp":
			checks = append(checks, NewTCPCheck(cc))
		case "dns":
			checks = append(checks, NewDNSCheck(cc))
		case "ping":
			checks = append(checks, NewPingCheck(cc))
		case "ssl":
			checks = append(checks, NewSSLCheck(cc))
		case "process":
			checks = append(checks, NewProcessCheck(cc))
		case "systemd":
			checks = append(checks, NewSystemdCheck(cc))
		case "file":
			checks = append(checks, NewFileCheck(cc))
		case "command":
			checks = append(checks, NewCommandCheck(cc))
		case "docker":
			checks = append(checks, NewDockerCheck(cc))
		case "ip":
			checks = append(checks, NewIPCheck(cc))
		case "log":
			checks = append(checks, NewLogCheck(cc))
		case "backup":
			checks = append(checks, NewBackupCheck(cc))
		case "smart":
			checks = append(checks, NewSmartCheck(cc))
		case "updates":
			checks = append(checks, NewUpdatesCheck(cc))
		case "fail2ban":
			checks = append(checks, NewFail2banCheck(cc))
		}
	}

	return checks
}

func setupHostEnv() {
	if _, err := os.Stat("/host/proc"); err == nil {
		os.Setenv("HOST_PROC", "/host/proc")
	}
	if _, err := os.Stat("/host/sys"); err == nil {
		os.Setenv("HOST_SYS", "/host/sys")
	}
}
