// Package notify delivers short alert messages to any of SMTP, a generic
// webhook, and ntfy. Sends are queued through one worker so a burst of
// detections cannot spawn unbounded goroutines or flood a provider. Test() is
// the synchronous variant used by the "send test" button.
package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
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
	Enabled   bool           `json:"enabled"`
	Events    []string       `json:"events"`     // event kinds that trigger a send; empty = all
	StaleDays int            `json:"stale_days"` // signatures-stale threshold; 0 => default 7
	SMTP      *SMTPConfig    `json:"smtp,omitempty"`
	Webhook   *WebhookConfig `json:"webhook,omitempty"`
	Ntfy      *NtfyConfig    `json:"ntfy,omitempty"`
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

type queued struct {
	kind, message string
}

// Dispatcher owns the current config and a single delivery worker.
type Dispatcher struct {
	mu     sync.RWMutex
	cfg    Config
	client *http.Client

	queue chan queued
	stop  chan struct{}
	wg    sync.WaitGroup
}

// New returns a running Dispatcher. Call Stop to release the worker.
func New(cfg Config) *Dispatcher {
	d := &Dispatcher{
		cfg:    cfg,
		client: &http.Client{Timeout: 15 * time.Second},
		queue:  make(chan queued, 256),
		stop:   make(chan struct{}),
	}
	d.wg.Add(1)
	go d.worker()
	return d
}

// Stop drains nothing but ends the worker goroutine.
func (d *Dispatcher) Stop() {
	close(d.stop)
	d.wg.Wait()
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()
	for {
		select {
		case <-d.stop:
			return
		case q := <-d.queue:
			if err := d.send(d.Config(), q.kind, q.message); err != nil {
				slog.Warn("notify: send failed", "kind", q.kind, "err", err)
			}
		}
	}
}

// Config returns the current configuration.
func (d *Dispatcher) Config() Config {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}

// SetConfig swaps in a new configuration. A nil sub-config (SMTP/Webhook/Ntfy)
// or a redacted/blank secret keeps whatever was stored before, so the redacted
// GET can round-trip without wiping credentials.
func (d *Dispatcher) SetConfig(next Config) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if next.SMTP == nil {
		next.SMTP = d.cfg.SMTP
	} else if d.cfg.SMTP != nil && (next.SMTP.Password == "" || next.SMTP.Password == "********") {
		next.SMTP.Password = d.cfg.SMTP.Password
	}
	if next.Webhook == nil {
		next.Webhook = d.cfg.Webhook
	}
	if next.Ntfy == nil {
		next.Ntfy = d.cfg.Ntfy
	} else if d.cfg.Ntfy != nil && (next.Ntfy.Token == "" || next.Ntfy.Token == "********") {
		next.Ntfy.Token = d.cfg.Ntfy.Token
	}
	d.cfg = next
}

// StaleDays is the configured signatures-stale threshold (>= 1).
func (d *Dispatcher) StaleDays() int {
	if n := d.Config().StaleDays; n >= 1 {
		return n
	}
	return 7
}

// Dispatch queues message for an event of kind. It is a no-op when
// notifications are disabled or kind is filtered out, and drops (with a log)
// rather than block when the queue is full.
func (d *Dispatcher) Dispatch(kind, message string) {
	cfg := d.Config()
	if !cfg.Enabled {
		return
	}
	if len(cfg.Events) > 0 && !contains(cfg.Events, kind) {
		return
	}
	select {
	case d.queue <- queued{kind: kind, message: message}:
	default:
		slog.Warn("notify: queue full, dropping alert", "kind", kind)
	}
}

// Test sends a fixed message to every configured channel synchronously.
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

// sendSMTP delivers one message. Port 465 => implicit TLS. StartTLS true =>
// upgrade the connection before auth. Otherwise a plain connection is used and
// auth is only attempted for localhost (to avoid leaking a password in clear).
func (d *Dispatcher) sendSMTP(c SMTPConfig, kind, message string) error {
	port := c.Port
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(port))
	msg := []byte(fmt.Sprintf("To: %s\r\nFrom: %s\r\nSubject: [ClamAV WebUI] %s\r\n\r\n%s\r\n",
		c.To, c.From, kind, message))
	rcpt := splitList(c.To)

	var conn net.Conn
	var err error
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	if port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: c.Host})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}

	cl, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer cl.Close()

	if port != 465 && c.StartTLS {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return errors.New("server does not offer STARTTLS")
		}
		if err := cl.StartTLS(&tls.Config{ServerName: c.Host}); err != nil {
			return err
		}
	}

	secure := port == 465 || c.StartTLS
	if c.Username != "" && (secure || c.Host == "localhost" || c.Host == "127.0.0.1") {
		if err := cl.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			return err
		}
	} else if c.Username != "" {
		return errors.New("refusing to send credentials over an unencrypted connection; enable StartTLS or use port 465")
	}

	if err := cl.Mail(c.From); err != nil {
		return err
	}
	for _, r := range rcpt {
		if err := cl.Rcpt(r); err != nil {
			return err
		}
	}
	wc, err := cl.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return cl.Quit()
}

func (d *Dispatcher) sendWebhook(url, kind, message string) error {
	payload, _ := json.Marshal(map[string]any{
		"kind": kind, "message": message, "time": time.Now().UTC().Format(time.RFC3339),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/"+c.Topic, strings.NewReader(message))
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
