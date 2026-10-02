package heed

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server      ServerConfig    `toml:"server"`
	Monitor     MonitorConfig   `toml:"monitor"`
	Alerts      AlertsConfig    `toml:"alerts"`
	CPU         ThresholdConfig `toml:"cpu"`
	Memory      ThresholdConfig `toml:"memory"`
	Swap        ThresholdConfig `toml:"swap"`
	Disk        DiskConfig      `toml:"disk"`
	Temperature ThresholdConfig `toml:"temperature"`
	Notify      []NotifyConfig  `toml:"notify"`
	Check       []CheckConfig   `toml:"check"`
}

type ServerConfig struct {
	Name string `toml:"name"`
}

type MonitorConfig struct {
	Interval     string `toml:"interval"`
	DataFile     string `toml:"data_file"`
	MaxDataSize  string `toml:"max_data_size"`
	StartupGrace string `toml:"startup_grace"`
}

type AlertsConfig struct {
	Cooldown       string `toml:"cooldown"`
	NotifyRecovery bool   `toml:"notify_recovery"`
}

type ThresholdConfig struct {
	Enabled  bool `toml:"enabled"`
	Warning  int  `toml:"warning"`
	Critical int  `toml:"critical"`
}

type DiskConfig struct {
	Enabled  bool   `toml:"enabled"`
	Path     string `toml:"path"`
	Warning  int    `toml:"warning"`
	Critical int    `toml:"critical"`
}

type NotifyConfig struct {
	Type       string `toml:"type"`
	Token      string `toml:"token"`
	ChatID     string `toml:"chat_id"`
	WebhookURL string `toml:"webhook_url"`
	URL        string `toml:"url"`
	Secret     string `toml:"secret"`
	Host       string `toml:"host"`
	Port       int    `toml:"port"`
	From       string `toml:"from"`
	To         string `toml:"to"`
	Username   string `toml:"username"`
	Password   string `toml:"password"`
}

type CheckConfig struct {
	Type           string            `toml:"type"`
	Name           string            `toml:"name"`
	URL            string            `toml:"url"`
	Host           string            `toml:"host"`
	Port           int               `toml:"port"`
	Timeout        string            `toml:"timeout"`
	ExpectedStatus int               `toml:"expected_status"`
	Warning        int               `toml:"warning"`
	Critical       int               `toml:"critical"`
	Match          string            `toml:"match"`
	MinCount       int               `toml:"min_count"`
	Path           string            `toml:"path"`
	MaxAge         string            `toml:"max_age"`
	Command        string            `toml:"command"`
	Container      string            `toml:"container"`
	DependsOn      []string          `toml:"depends_on"`
	Method         string            `toml:"method"`
	Headers        map[string]string `toml:"headers"`
	Body           string            `toml:"body"`
	MaxLatency     string            `toml:"max_latency"`
	ExpectBody     string            `toml:"expect_body"`
	ExpectJSON     map[string]any    `toml:"expect_json"`
	Pattern        string            `toml:"pattern"`
	IgnoreCase     bool              `toml:"ignore_case"`
	MaxCPU         int               `toml:"max_cpu"`
	MaxMemory      string            `toml:"max_memory"`
	State          string            `toml:"state"`
	Device         string            `toml:"device"`
	Jail           string            `toml:"jail"`
	PackageManager string            `toml:"package_manager"`
}

func (c *MonitorConfig) IntervalDuration() time.Duration {
	d, err := time.ParseDuration(c.Interval)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

func (c *MonitorConfig) StartupGraceDuration() time.Duration {
	if c.StartupGrace == "" {
		return 0
	}
	d, err := time.ParseDuration(c.StartupGrace)
	if err != nil {
		return 0
	}
	return d
}

func (c *AlertsConfig) CooldownDuration() time.Duration {
	d, err := time.ParseDuration(c.Cooldown)
	if err != nil {
		return 15 * time.Minute
	}
	return d
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Name == "" {
		hostname, _ := os.Hostname()
		if hostname != "" {
			c.Server.Name = hostname
		} else {
			c.Server.Name = "server"
		}
	}
	if c.Monitor.Interval == "" {
		c.Monitor.Interval = "10s"
	}
	if c.Monitor.DataFile == "" {
		c.Monitor.DataFile = "events.jsonl"
	}
	if c.Monitor.MaxDataSize == "" {
		c.Monitor.MaxDataSize = "50MB"
	}
	if c.Alerts.Cooldown == "" {
		c.Alerts.Cooldown = "15m"
	}
	if c.Disk.Path == "" && c.Disk.Enabled {
		c.Disk.Path = "/"
	}
	if len(c.Notify) == 0 {
		c.Notify = []NotifyConfig{{Type: "stdout"}}
	}
}

func ValidateConfig(cfg Config) []string {
	var errs []string

	if _, err := time.ParseDuration(cfg.Monitor.Interval); err != nil {
		errs = append(errs, fmt.Sprintf("monitor.interval: %v", err))
	}
	if _, err := time.ParseDuration(cfg.Alerts.Cooldown); err != nil {
		errs = append(errs, fmt.Sprintf("alerts.cooldown: %v", err))
	}

	for i, n := range cfg.Notify {
		switch n.Type {
		case "stdout", "webhook", "discord", "slack", "telegram", "smtp":
		default:
			errs = append(errs, fmt.Sprintf("notify[%d]: unknown type %q", i, n.Type))
		}
	}

	for i, c := range cfg.Check {
		if c.Name == "" {
			errs = append(errs, fmt.Sprintf("check[%d]: name is required", i))
		}
		switch c.Type {
		case "http", "tcp", "dns", "ping", "ssl", "process", "systemd", "file", "command", "docker", "ip", "log", "backup", "smart", "updates", "fail2ban":
		default:
			errs = append(errs, fmt.Sprintf("check[%d]: unknown type %q", i, c.Type))
		}
	}

	return errs
}
