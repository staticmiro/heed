package heed

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

type Notifier interface {
	Name() string
	Send(event Event, serverName string) error
}

type stdoutNotifier struct{}

func (n *stdoutNotifier) Name() string { return "stdout" }

func (n *stdoutNotifier) Send(event Event, serverName string) error {
	icon := stateIcon(event.State)
	fmt.Printf("[%s %s] %s: %s (was: %s)\n",
		icon,
		strings.ToUpper(string(event.State)),
		event.Check,
		event.Message,
		event.PrevState,
	)
	return nil
}

type telegramNotifier struct {
	token  string
	chatID string
	client *http.Client
}

func (n *telegramNotifier) Name() string { return "telegram" }

func (n *telegramNotifier) Send(event Event, serverName string) error {
	text := formatAlert(event, serverName)
	payload, _ := json.Marshal(map[string]string{
		"chat_id": n.chatID,
		"text":    text,
	})

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.token)
	resp, err := n.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram: %s", resp.Status)
	}
	return nil
}

type discordNotifier struct {
	webhookURL string
	client     *http.Client
}

func (n *discordNotifier) Name() string { return "discord" }

func (n *discordNotifier) Send(event Event, serverName string) error {
	text := formatAlert(event, serverName)
	payload, _ := json.Marshal(map[string]string{"content": text})
	return postWebhook(n.client, n.webhookURL, payload, "")
}

type slackNotifier struct {
	webhookURL string
	client     *http.Client
}

func (n *slackNotifier) Name() string { return "slack" }

func (n *slackNotifier) Send(event Event, serverName string) error {
	text := formatAlert(event, serverName)
	payload, _ := json.Marshal(map[string]string{"text": text})
	return postWebhook(n.client, n.webhookURL, payload, "")
}

type webhookNotifier struct {
	url    string
	secret string
	client *http.Client
}

func (n *webhookNotifier) Name() string { return "webhook" }

func (n *webhookNotifier) Send(event Event, serverName string) error {
	payload, _ := json.Marshal(map[string]any{
		"server":     serverName,
		"check":      event.Check,
		"state":      event.State,
		"prev_state": event.PrevState,
		"value":      event.Value,
		"message":    event.Message,
		"timestamp":  event.Timestamp,
	})
	return postWebhook(n.client, n.url, payload, n.secret)
}

type smtpNotifier struct {
	host     string
	port     int
	from     string
	to       string
	username string
	password string
}

func (n *smtpNotifier) Name() string { return "smtp" }

func (n *smtpNotifier) Send(event Event, serverName string) error {
	text := formatAlert(event, serverName)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: [HEED] %s - %s\r\n\r\n%s",
		n.from, n.to, strings.ToUpper(string(event.State)), event.Check, text)

	addr := fmt.Sprintf("%s:%d", n.host, n.port)
	var auth smtp.Auth
	if n.username != "" {
		auth = smtp.PlainAuth("", n.username, n.password, n.host)
	}

	return smtp.SendMail(addr, auth, n.from, []string{n.to}, []byte(msg))
}

func postWebhook(client *http.Client, url string, body []byte, secret string) error {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Signature", "sha256="+sig)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook %s: %s", url, resp.Status)
	}
	return nil
}

func formatAlert(event Event, serverName string) string {
	icon := stateIcon(event.State)
	label := strings.ToUpper(string(event.State))
	return fmt.Sprintf("%s %s — %s\n%s: %s\nwas: %s",
		icon, label, serverName,
		event.Check, event.Message,
		event.PrevState,
	)
}

func stateIcon(s State) string {
	switch s {
	case StateOK:
		return "+"
	case StateWarning:
		return "!"
	case StateCritical:
		return "*"
	default:
		return "?"
	}
}

func BuildNotifiers(cfg Config) []Notifier {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	var notifiers []Notifier

	for _, nc := range cfg.Notify {
		switch nc.Type {
		case "stdout":
			notifiers = append(notifiers, &stdoutNotifier{})
		case "telegram":
			notifiers = append(notifiers, &telegramNotifier{
				token:  nc.Token,
				chatID: nc.ChatID,
				client: httpClient,
			})
		case "discord":
			notifiers = append(notifiers, &discordNotifier{
				webhookURL: nc.WebhookURL,
				client:     httpClient,
			})
		case "slack":
			notifiers = append(notifiers, &slackNotifier{
				webhookURL: nc.WebhookURL,
				client:     httpClient,
			})
		case "webhook":
			notifiers = append(notifiers, &webhookNotifier{
				url:    nc.URL,
				secret: nc.Secret,
				client: httpClient,
			})
		case "smtp":
			notifiers = append(notifiers, &smtpNotifier{
				host:     nc.Host,
				port:     nc.Port,
				from:     nc.From,
				to:       nc.To,
				username: nc.Username,
				password: nc.Password,
			})
		}
	}

	if len(notifiers) == 0 {
		notifiers = append(notifiers, &stdoutNotifier{})
	}
	return notifiers
}
