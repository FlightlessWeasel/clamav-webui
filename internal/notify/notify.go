// Package notify delivers short alert messages to any of SMTP, a generic
// webhook, and ntfy. Sending is best-effort and asynchronous; Test() is the
// synchronous variant used by the "send test" button.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// SMTPConfig configures email delivery.
type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
	StartTLS bool   `json:"starttls"`
}

// WebhookConfig posts a JSON body to URL.
type WebhookConfig struct {
	URL string `json:"url"`
}

// NtfyConfig publishes to an ntfy topic.
type NtfyConfig struct {
	BaseURL string `json:"base_url"` // default https://ntfy.sh
	Topic   string `json:"topic"`
	Token   string `json:"token"`
}

// Config is the full notification configuration (persisted in settings).
type Config struct {
	Enabled bool           `json:"enabled"`
	Events  []string       `json:"events"` // event kinds that trigger a send; empty = all
	SMTP    *SMTPConfig    `json:"smtp,omitempty"`
	Webhook *WebhookConfig `json:"webhook,omitempty"`
	Ntfy    *NtfyConfig    `json:"ntfy,omitempty"`
}

// Redacted returns a copy with secrets blanked, for GET responses.
func (c Config) Redacted() Config {
	if c.SMTP != nil {
		cp := *c.SMTP
		if cp.Password != "" {
			cp.Password = "********"
		}
		c.SMTP = &cp
	}
	if c.Ntfy != nil {
		cp := *c.Ntfy
		if cp.Token != "" {
			cp.Token = "********"
		}
		c.Ntfy = &cp
	}
	return c
}

// Dispatcher owns the current config and sends messages.
type Dispatcher struct {
	mu     sync.RWMutex
	cfg    Config
	client *http.Client
}

// New returns a Dispatcher with the given initial config.
func New(cfg Config) *Dispatcher {
	return &Dispatcher{cfg: cfg, client: &http.Client{Timeout: 15 * time.Second}}
}

// Config returns the current configuration.
func (d *Dispatcher) Config() Config {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}

// SetConfig swaps in a new configuration. A nil/empty secret on a sub-config
// keeps the previously stored secret (so the redacted GET can round-trip).
func (d *Dispatcher) SetConfig(next Config) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if next.SMTP != nil && d.cfg.SMTP != nil && (next.SMTP.Password == "" || next.SMTP.Password == "********") {
		next.SMTP.Password = d.cfg.SMTP.Password
	}
	if next.Ntfy != nil && d.cfg.Ntfy != nil && (next.Ntfy.Token == "" || next.Ntfy.Token == "********") {
		next.Ntfy.Token = d.cfg.Ntfy.Token
	}
	d.cfg = next
}

// Dispatch sends message for an event of kind, asynchronously. It is a no-op
// when notifications are disabled or kind is filtered out.
func (d *Dispatcher) Dispatch(kind, message string) {
	cfg := d.Config()
	if !cfg.Enabled {
		return
	}
	if len(cfg.Events) > 0 && !contains(cfg.Events, kind) {
		return
	}
	go func() {
		if err := d.send(cfg, kind, message); err != nil {
			slog.Warn("notify: dispatch failed", "kind", kind, "err", err)
		}
	}()
}

// Test sends a fixed message to every configured channel synchronously and
// returns the first error.
func (d *Dispatcher) Test() error {
	cfg := d.Config()
	if !hasChannel(cfg) {
		return errors.New("no notification channel is configured")
	}
	return d.send(cfg, "test", "ClamAV WebUI test notification — if you see this, alerts are working.")
}

func (d *Dispatcher) send(cfg Config, kind, message string) error {
	var errs []string
	if cfg.SMTP != nil && cfg.SMTP.Host != "" {
		if err := d.sendSMTP(*cfg.SMTP, kind, message); err != nil {
			errs = append(errs, "smtp: "+err.Error())
		}
	}
	if cfg.Webhook != nil && cfg.Webhook.URL != "" {
		if err := d.sendWebhook(cfg.Webhook.URL, kind, message); err != nil {
			errs = append(errs, "webhook: "+err.Error())
		}
	}
	if cfg.Ntfy != nil && cfg.Ntfy.Topic != "" {
		if err := d.sendNtfy(*cfg.Ntfy, kind, message); err != nil {
			errs = append(errs, "ntfy: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (d *Dispatcher) sendSMTP(c SMTPConfig, kind, message string) error {
	addr := net.JoinHostPort(c.Host, itoa(c.Port, 587))
	body := fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: [ClamAV WebUI] %s\r\n\r\n%s\r\n",
		c.To, c.From, kind, message)

	var auth smtp.Auth
	if c.Username != "" {
		auth = smtp.PlainAuth("", c.Username, c.Password, c.Host)
	}
	return smtp.SendMail(addr, auth, c.From, splitList(c.To), []byte(body))
}

func (d *Dispatcher) sendWebhook(url, kind, message string) error {
	payload, _ := json.Marshal(map[string]any{
		"kind": kind, "message": message, "time": time.Now().UTC().Format(time.RFC3339),
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return d.do(req)
}

func (d *Dispatcher) sendNtfy(c NtfyConfig, kind, message string) error {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://ntfy.sh"
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/"+c.Topic, strings.NewReader(message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", "ClamAV WebUI: "+kind)
	if kind == "onaccess-detection" || kind == "scan-detection" {
		req.Header.Set("Priority", "urgent")
		req.Header.Set("Tags", "warning")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return d.do(req)
}

func (d *Dispatcher) do(req *http.Request) error {
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func hasChannel(c Config) bool {
	return (c.SMTP != nil && c.SMTP.Host != "") ||
		(c.Webhook != nil && c.Webhook.URL != "") ||
		(c.Ntfy != nil && c.Ntfy.Topic != "")
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func itoa(n, def int) string {
	if n <= 0 {
		n = def
	}
	return fmt.Sprintf("%d", n)
}
