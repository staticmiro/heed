package heed

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type httpCheck struct {
	name           string
	url            string
	method         string
	headers        map[string]string
	body           string
	expectedStatus int
	timeout        time.Duration
	warningMs      int
	criticalMs     int
	maxLatency     time.Duration
	expectBody     string
	expectJSON     map[string]any
}

func NewHTTPCheck(cfg CheckConfig) Checker {
	expectedStatus := cfg.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = 200
	}
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = "GET"
	}
	var maxLatency time.Duration
	if cfg.MaxLatency != "" {
		maxLatency, _ = time.ParseDuration(cfg.MaxLatency)
	}
	return &httpCheck{
		name:           cfg.Name,
		url:            cfg.URL,
		method:         method,
		headers:        cfg.Headers,
		body:           cfg.Body,
		expectedStatus: expectedStatus,
		timeout:        parseDurationOrDefault(cfg.Timeout, 10*time.Second),
		warningMs:      cfg.Warning,
		criticalMs:     cfg.Critical,
		maxLatency:     maxLatency,
		expectBody:     cfg.ExpectBody,
		expectJSON:     cfg.ExpectJSON,
	}
}

func (c *httpCheck) Name() string { return c.name }

func (c *httpCheck) Check() Result {
	client := &http.Client{Timeout: c.timeout}

	var reqBody io.Reader
	if c.body != "" {
		reqBody = bytes.NewBufferString(c.body)
	}

	req, err := http.NewRequest(c.method, c.url, reqBody)
	if err != nil {
		return Result{Name: c.name, State: StateCritical, Value: "req error", Message: err.Error()}
	}

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "error",
			Message: fmt.Sprintf("%s: %v", c.url, err),
		}
	}
	defer resp.Body.Close()

	ms := int(elapsed.Milliseconds())

	if resp.StatusCode != c.expectedStatus {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   fmt.Sprintf("%d %dms", resp.StatusCode, ms),
			Numeric: float64(ms),
			Message: fmt.Sprintf("%s returned %d, expected %d", c.url, resp.StatusCode, c.expectedStatus),
		}
	}

	if c.maxLatency > 0 && elapsed > c.maxLatency {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   fmt.Sprintf("latency %dms", ms),
			Numeric: float64(ms),
			Message: fmt.Sprintf("latency %s exceeded max %s", elapsed, c.maxLatency),
		}
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	if c.expectBody != "" && !strings.Contains(bodyStr, c.expectBody) {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "body mismatch",
			Numeric: float64(ms),
			Message: fmt.Sprintf("body did not contain %q", truncate(c.expectBody, 50)),
		}
	}

	if len(c.expectJSON) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
			return Result{
				Name:    c.name,
				State:   StateCritical,
				Value:   "json error",
				Numeric: float64(ms),
				Message: fmt.Sprintf("failed to parse json: %v", err),
			}
		}
		for k, v := range c.expectJSON {
			if parsedVal, ok := parsed[k]; !ok || fmt.Sprintf("%v", parsedVal) != fmt.Sprintf("%v", v) {
				return Result{
					Name:    c.name,
					State:   StateCritical,
					Value:   "json mismatch",
					Numeric: float64(ms),
					Message: fmt.Sprintf("json field %q mismatch: expected %v, got %v", k, v, parsedVal),
				}
			}
		}
	}

	state := StateOK
	if c.criticalMs > 0 && ms >= c.criticalMs {
		state = StateCritical
	} else if c.warningMs > 0 && ms >= c.warningMs {
		state = StateWarning
	}

	return Result{
		Name:    c.name,
		State:   state,
		Value:   fmt.Sprintf("%dms", ms),
		Numeric: float64(ms),
		Message: fmt.Sprintf("%s OK in %dms", c.url, ms),
	}
}

type tcpCheck struct {
	name    string
	host    string
	port    int
	timeout time.Duration
}

func NewTCPCheck(cfg CheckConfig) Checker {
	return &tcpCheck{
		name:    cfg.Name,
		host:    cfg.Host,
		port:    cfg.Port,
		timeout: parseDurationOrDefault(cfg.Timeout, 5*time.Second),
	}
}

func (c *tcpCheck) Name() string { return c.name }

func (c *tcpCheck) Check() Result {
	addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))

	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, c.timeout)
	elapsed := time.Since(start)

	if err != nil {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   fmt.Sprintf("tcp/%d", c.port),
			Message: fmt.Sprintf("%s: %v", addr, err),
		}
	}
	conn.Close()

	ms := int(elapsed.Milliseconds())
	return Result{
		Name:    c.name,
		State:   StateOK,
		Value:   fmt.Sprintf("tcp/%d %dms", c.port, ms),
		Numeric: float64(ms),
		Message: fmt.Sprintf("%s reachable in %dms", addr, ms),
	}
}

type dnsCheck struct {
	name    string
	host    string
	timeout time.Duration
}

func NewDNSCheck(cfg CheckConfig) Checker {
	return &dnsCheck{
		name:    cfg.Name,
		host:    cfg.Host,
		timeout: parseDurationOrDefault(cfg.Timeout, 5*time.Second),
	}
}

func (c *dnsCheck) Name() string { return c.name }

func (c *dnsCheck) Check() Result {
	resolver := &net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	start := time.Now()
	addrs, err := resolver.LookupHost(ctx, c.host)
	elapsed := time.Since(start)

	if err != nil {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "error",
			Message: fmt.Sprintf("DNS %s: %v", c.host, err),
		}
	}

	ms := int(elapsed.Milliseconds())
	return Result{
		Name:    c.name,
		State:   StateOK,
		Value:   fmt.Sprintf("%dms %s", ms, addrs[0]),
		Numeric: float64(ms),
		Message: fmt.Sprintf("DNS %s → %s in %dms", c.host, strings.Join(addrs, ", "), ms),
	}
}

type pingCheck struct {
	name    string
	host    string
	timeout time.Duration
}

func NewPingCheck(cfg CheckConfig) Checker {
	return &pingCheck{
		name:    cfg.Name,
		host:    cfg.Host,
		timeout: parseDurationOrDefault(cfg.Timeout, 5*time.Second),
	}
}

func (c *pingCheck) Name() string { return c.name }

func (c *pingCheck) Check() Result {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("ping", "-c", "1", "-W", strconv.Itoa(int(c.timeout.Milliseconds())), c.host)
	default:
		sec := int(c.timeout.Seconds())
		if sec < 1 {
			sec = 1
		}
		cmd = exec.Command("ping", "-c", "1", "-W", strconv.Itoa(sec), c.host)
	}

	start := time.Now()
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "unreachable",
			Message: fmt.Sprintf("Ping %s failed", c.host),
		}
	}

	ms := elapsed.Milliseconds()
	if rtt := extractPingRTT(string(output)); rtt > 0 {
		ms = int64(rtt)
	}

	return Result{
		Name:    c.name,
		State:   StateOK,
		Value:   fmt.Sprintf("%dms", ms),
		Numeric: float64(ms),
		Message: fmt.Sprintf("Ping %s: %dms", c.host, ms),
	}
}

func extractPingRTT(output string) float64 {
	for _, line := range strings.Split(output, "\n") {
		idx := strings.Index(line, "time=")
		if idx < 0 {
			continue
		}
		part := line[idx+5:]
		if sp := strings.IndexAny(part, " \t"); sp > 0 {
			part = part[:sp]
		}
		val, err := strconv.ParseFloat(part, 64)
		if err == nil {
			return val
		}
	}
	return 0
}

type sslCheck struct {
	name     string
	host     string
	port     int
	warning  int
	critical int
	timeout  time.Duration
}

func NewSSLCheck(cfg CheckConfig) Checker {
	port := cfg.Port
	if port == 0 {
		port = 443
	}
	warning := cfg.Warning
	if warning == 0 {
		warning = 30
	}
	critical := cfg.Critical
	if critical == 0 {
		critical = 7
	}
	return &sslCheck{
		name:     cfg.Name,
		host:     cfg.Host,
		port:     port,
		warning:  warning,
		critical: critical,
		timeout:  parseDurationOrDefault(cfg.Timeout, 10*time.Second),
	}
}

func (c *sslCheck) Name() string { return c.name }

func (c *sslCheck) Check() Result {
	addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: c.timeout},
		"tcp", addr,
		&tls.Config{ServerName: c.host},
	)
	if err != nil {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "error",
			Message: fmt.Sprintf("TLS %s: %v", addr, err),
		}
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Result{Name: c.name, State: StateUnknown, Value: "no cert", Message: "No certificate found"}
	}

	expiry := certs[0].NotAfter
	daysLeft := int(time.Until(expiry).Hours() / 24)

	state := StateOK
	if daysLeft <= c.critical {
		state = StateCritical
	} else if daysLeft <= c.warning {
		state = StateWarning
	}

	return Result{
		Name:    c.name,
		State:   state,
		Value:   fmt.Sprintf("%dd", daysLeft),
		Numeric: float64(daysLeft),
		Message: fmt.Sprintf("SSL %s expires in %d days (%s)", c.host, daysLeft, expiry.Format("2006-01-02")),
	}
}

func parseDurationOrDefault(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

type ipCheck struct {
	name    string
	url     string
	timeout time.Duration
}

func NewIPCheck(cfg CheckConfig) Checker {
	url := cfg.URL
	if url == "" {
		url = "https://api.ipify.org"
	}
	return &ipCheck{
		name:    cfg.Name,
		url:     url,
		timeout: parseDurationOrDefault(cfg.Timeout, 10*time.Second),
	}
}

func (c *ipCheck) Name() string { return c.name }

func (c *ipCheck) Check() Result {
	client := &http.Client{Timeout: c.timeout}

	resp, err := client.Get(c.url)
	if err != nil {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "error",
			Message: fmt.Sprintf("IP check failed: %v", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "error",
			Message: fmt.Sprintf("IP check returned %d", resp.StatusCode),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "error",
			Message: fmt.Sprintf("IP check read failed: %v", err),
		}
	}

	ip := strings.TrimSpace(string(body))
	if net.ParseIP(ip) == nil {
		return Result{
			Name:    c.name,
			State:   StateCritical,
			Value:   "invalid",
			Message: fmt.Sprintf("IP check returned invalid IP: %s", ip),
		}
	}

	return Result{
		Name:    c.name,
		State:   StateOK,
		Value:   ip,
		Message: fmt.Sprintf("Public IP: %s", ip),
	}
}
