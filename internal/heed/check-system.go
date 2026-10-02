package heed

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

type cpuCheck struct {
	warning  int
	critical int
}

func NewCPUCheck(warning, critical int) Checker {
	return &cpuCheck{warning: warning, critical: critical}
}

func (c *cpuCheck) Name() string { return "cpu" }

func (c *cpuCheck) Check() Result {
	percents, err := cpu.Percent(0, false)
	if err != nil || len(percents) == 0 {
		return Result{Name: "cpu", State: StateUnknown, Message: fmt.Sprintf("read error: %v", err)}
	}

	pct := int(percents[0])
	return Result{
		Name:      "cpu",
		State:     evaluateThreshold(pct, c.warning, c.critical),
		Value:     fmt.Sprintf("%d%%", pct),
		Numeric:   percents[0],
		Threshold: float64(c.critical),
		Message:   fmt.Sprintf("CPU usage is %d%%", pct),
	}
}

type memoryCheck struct {
	warning  int
	critical int
}

func NewMemoryCheck(warning, critical int) Checker {
	return &memoryCheck{warning: warning, critical: critical}
}

func (c *memoryCheck) Name() string { return "memory" }

func (c *memoryCheck) Check() Result {
	info, err := mem.VirtualMemory()
	if err != nil {
		return Result{Name: "memory", State: StateUnknown, Message: fmt.Sprintf("read error: %v", err)}
	}

	pct := int(info.UsedPercent)
	return Result{
		Name:      "memory",
		State:     evaluateThreshold(pct, c.warning, c.critical),
		Value:     fmt.Sprintf("%d%%", pct),
		Numeric:   info.UsedPercent,
		Threshold: float64(c.critical),
		Message:   fmt.Sprintf("Memory usage is %d%%", pct),
	}
}

type swapCheck struct {
	warning  int
	critical int
}

func NewSwapCheck(warning, critical int) Checker {
	return &swapCheck{warning: warning, critical: critical}
}

func (c *swapCheck) Name() string { return "swap" }

func (c *swapCheck) Check() Result {
	info, err := mem.SwapMemory()
	if err != nil {
		return Result{Name: "swap", State: StateUnknown, Message: fmt.Sprintf("read error: %v", err)}
	}

	if info.Total == 0 {
		return Result{Name: "swap", State: StateOK, Value: "N/A", Message: "No swap configured"}
	}

	pct := int(info.UsedPercent)
	return Result{
		Name:      "swap",
		State:     evaluateThreshold(pct, c.warning, c.critical),
		Value:     fmt.Sprintf("%d%%", pct),
		Numeric:   info.UsedPercent,
		Threshold: float64(c.critical),
		Message:   fmt.Sprintf("Swap usage is %d%%", pct),
	}
}

type diskCheck struct {
	path     string
	warning  int
	critical int
}

func NewDiskCheck(path string, warning, critical int) Checker {
	return &diskCheck{path: path, warning: warning, critical: critical}
}

func (c *diskCheck) Name() string { return "disk" }

func (c *diskCheck) Check() Result {
	info, err := disk.Usage(c.path)
	if err != nil {
		return Result{Name: "disk", State: StateUnknown, Message: fmt.Sprintf("read error: %v", err)}
	}

	pct := int(info.UsedPercent)
	return Result{
		Name:      "disk",
		State:     evaluateThreshold(pct, c.warning, c.critical),
		Value:     fmt.Sprintf("%d%%", pct),
		Numeric:   info.UsedPercent,
		Threshold: float64(c.critical),
		Message:   fmt.Sprintf("Disk usage is %d%% on %s", pct, c.path),
	}
}

type temperatureCheck struct {
	warning  int
	critical int
}

func NewTemperatureCheck(warning, critical int) Checker {
	return &temperatureCheck{warning: warning, critical: critical}
}

func (c *temperatureCheck) Name() string { return "temperature" }

func (c *temperatureCheck) Check() Result {
	temp := readTemp()
	if temp <= 0 {
		return Result{Name: "temperature", State: StateOK, Value: "N/A", Message: "No temperature sensors"}
	}

	t := int(temp)
	return Result{
		Name:      "temperature",
		State:     evaluateThreshold(t, c.warning, c.critical),
		Value:     fmt.Sprintf("%d°C", t),
		Numeric:   temp,
		Threshold: float64(c.critical),
		Message:   fmt.Sprintf("Temperature is %d°C", t),
	}
}

func readTemp() float64 {
	temps, err := host.SensorsTemperatures()
	if err == nil && len(temps) > 0 {
		var maxTemp float64
		for _, t := range temps {
			if t.Temperature > maxTemp && t.Temperature > 0 {
				maxTemp = t.Temperature
			}
		}
		if maxTemp > 0 {
			return maxTemp
		}
	}

	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0
	}

	value, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return 0
	}

	return value / 1000.0
}

type loadCheck struct {
	warning  float64
	critical float64
}

func NewLoadCheck() Checker {
	cores := float64(runtime.NumCPU())
	return &loadCheck{warning: cores, critical: cores * 1.5}
}

func (c *loadCheck) Name() string { return "load" }

func (c *loadCheck) Check() Result {
	avg, err := load.Avg()
	if err != nil {
		return Result{Name: "load", State: StateUnknown, Message: fmt.Sprintf("read error: %v", err)}
	}

	state := StateOK
	if avg.Load1 >= c.critical {
		state = StateCritical
	} else if avg.Load1 >= c.warning {
		state = StateWarning
	}

	return Result{
		Name:    "load",
		State:   state,
		Value:   fmt.Sprintf("%.2f", avg.Load1),
		Numeric: avg.Load1,
		Message: fmt.Sprintf("Load %.2f / %.2f / %.2f", avg.Load1, avg.Load5, avg.Load15),
	}
}

type uptimeCheck struct{}

func NewUptimeCheck() Checker { return &uptimeCheck{} }

func (c *uptimeCheck) Name() string { return "uptime" }

func (c *uptimeCheck) Check() Result {
	uptime, err := host.Uptime()
	if err != nil {
		return Result{Name: "uptime", State: StateUnknown, Message: fmt.Sprintf("read error: %v", err)}
	}

	days := uptime / 86400
	hours := (uptime % 86400) / 3600
	return Result{
		Name:    "uptime",
		State:   StateOK,
		Value:   fmt.Sprintf("%dd %dh", days, hours),
		Numeric: float64(uptime),
		Message: fmt.Sprintf("Uptime: %d days, %d hours", days, hours),
	}
}

func evaluateThreshold(value, warning, critical int) State {
	if value >= critical {
		return StateCritical
	}
	if value >= warning {
		return StateWarning
	}
	return StateOK
}
