package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"heed/internal/heed"
)

var version = "dev"

func main() {
	configPath := "config.toml"
	historyLimit := 20
	var cmd string

	i := 1
	for i < len(os.Args) {
		switch os.Args[i] {
		case "-config":
			i++
			if i < len(os.Args) {
				configPath = os.Args[i]
			}
		case "-n":
			i++
			if i < len(os.Args) {
				if n, err := strconv.Atoi(os.Args[i]); err == nil && n > 0 {
					historyLimit = n
				}
			}
		case "-help", "--help":
			cmd = "help"
		default:
			if cmd == "" && !strings.HasPrefix(os.Args[i], "-") {
				cmd = os.Args[i]
			}
		}
		i++
	}

	switch cmd {
	case "":
		runDaemon(configPath)
	case "version":
		fmt.Printf("heed %s\n", version)
	case "validate":
		runValidate(configPath)
	case "status", "check":
		runStatus(configPath)
	case "history":
		runHistory(configPath, historyLimit)
	case "test":
		runTest(configPath)
	case "silence":
		runSilence(configPath)
	case "help":
		runHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

func runDaemon(configPath string) {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	cfg, err := heed.LoadConfig(configPath)
	if err != nil {
		logger.Fatalf("config: %v", err)
	}

	if errs := heed.ValidateConfig(cfg); len(errs) > 0 {
		for _, e := range errs {
			logger.Printf("config error: %s", e)
		}
		os.Exit(1)
	}

	monitor, err := heed.NewMonitor(cfg, logger)
	if err != nil {
		logger.Fatalf("monitor: %v", err)
	}
	defer monitor.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Println("shutting down")
		cancel()
	}()

	monitor.Run(ctx)
}

func runValidate(configPath string) {
	cfg, err := heed.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	errs := heed.ValidateConfig(cfg)
	if len(errs) == 0 {
		fmt.Println("config is valid")
		return
	}

	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "  %s\n", e)
	}
	os.Exit(1)
}

func runStatus(configPath string) {
	cfg, err := heed.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	logger := log.New(os.Stderr, "", 0)
	monitor, err := heed.NewMonitor(cfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "monitor: %v\n", err)
		os.Exit(1)
	}
	defer monitor.Close()

	statuses := monitor.Status()

	overallState := heed.StateOK
	for _, s := range statuses {
		if s.State == heed.StateCritical {
			overallState = heed.StateCritical
		}
		if s.State == heed.StateWarning && overallState != heed.StateCritical {
			overallState = heed.StateWarning
		}
	}

	fmt.Printf("Server: %s\n", cfg.Server.Name)
	fmt.Printf("Status: %s\n\n", strings.ToUpper(string(overallState)))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	for _, s := range statuses {
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, strings.ToUpper(string(s.State)), s.Value)
	}
	w.Flush()
}

func runHistory(configPath string, limit int) {
	cfg, err := heed.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	events, err := heed.ReadEvents(cfg.Monitor.DataFile, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read events: %v\n", err)
		os.Exit(1)
	}

	if len(events) == 0 {
		fmt.Println("no events")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	for _, e := range events {
		fmt.Fprintf(w, "%s\t%s\t%s → %s\t%s\n",
			e.Timestamp.Format("2006-01-02 15:04:05"),
			e.Check,
			e.PrevState, e.State,
			e.Value,
		)
	}
	w.Flush()
}

func runTest(configPath string) {
	cfg, err := heed.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	notifiers := heed.BuildNotifiers(cfg)
	if len(notifiers) == 0 {
		fmt.Println("no notifiers configured")
		return
	}

	event := heed.Event{
		Timestamp: time.Now().UTC(),
		Check:     "heed_test",
		PrevState: heed.StateOK,
		State:     heed.StateCritical,
		Value:     "test",
		Message:   "This is a test notification from heed",
	}

	fmt.Printf("Sending test notification to %d channels...\n", len(notifiers))
	for _, n := range notifiers {
		err := n.Send(event, cfg.Server.Name)
		if err != nil {
			fmt.Printf("[FAIL] %s: error: %v\n", n.Name(), err)
		} else {
			fmt.Printf("[OK] %s: sent successfully\n", n.Name())
		}
	}
}

func runHelp() {
	fmt.Println("Usage: heed [command] [-config config.toml]")
	fmt.Println("Commands:")
	fmt.Println("  (empty)  Run the monitoring daemon")
	fmt.Println("  status   Print current status of all checks")
	fmt.Println("  history  Print event history")
	fmt.Println("  test     Send a test notification")
	fmt.Println("  validate Validate configuration")
	fmt.Println("  silence  Silence alerts")
	fmt.Println("  version  Print version")
	fmt.Println("  help     Print this help message")
}

func runSilence(configPath string) {
	cfg, err := heed.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	args := []string{}
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "silence" {
			for j := i + 1; j < len(os.Args); j++ {
				if os.Args[j] == "-config" || os.Args[j] == "-n" {
					j++ // skip flag value
					continue
				}
				if !strings.HasPrefix(os.Args[j], "-") {
					args = append(args, os.Args[j])
				}
			}
			break
		}
	}

	if len(args) == 0 {
		fmt.Println("usage: heed silence [check_name] <duration>")
		os.Exit(1)
	}

	var durationStr string
	var checkName string

	if len(args) == 1 {
		durationStr = args[0]
	} else {
		checkName = args[0]
		durationStr = args[1]
	}

	d, err := time.ParseDuration(durationStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid duration %q: %v\n", durationStr, err)
		os.Exit(1)
	}

	sm := heed.NewSilenceManager(cfg.Monitor.DataFile)
	err = sm.AddRule(heed.SilenceRule{
		Check: checkName,
		Until: time.Now().Add(d),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to add silence rule: %v\n", err)
		os.Exit(1)
	}

	if checkName == "" {
		fmt.Printf("Silenced ALL alerts for %s\n", d)
	} else {
		fmt.Printf("Silenced alerts for check %q for %s\n", checkName, d)
	}
}
