package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatchDisabledIsNoop(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	d := New(Config{Enabled: false, Webhook: &WebhookConfig{URL: srv.URL}})
	d.Dispatch("scan-detection", "x")
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatal("disabled dispatcher should not send")
	}
}

func TestDispatchEventFilter(t *testing.T) {
	got := make(chan map[string]any, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		json.Unmarshal(b, &m)
		got <- m
	}))
	defer srv.Close()

	d := New(Config{Enabled: true, Events: []string{"scan-detection"}, Webhook: &WebhookConfig{URL: srv.URL}})

	d.Dispatch("signatures-stale", "ignored")
	d.Dispatch("scan-detection", "delivered")

	select {
	case m := <-got:
		if m["message"] != "delivered" {
			t.Fatalf("got %v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("expected the allowed event to be delivered")
	}
	select {
	case m := <-got:
		t.Fatalf("filtered event was delivered: %v", m)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestTestReturnsErrorWithNoChannel(t *testing.T) {
	if err := New(Config{Enabled: true}).Test(); err == nil {
		t.Fatal("expected an error when nothing is configured")
	}
}

func TestSetConfigKeepsExistingSecret(t *testing.T) {
	d := New(Config{SMTP: &SMTPConfig{Host: "mail", Password: "s3cret"}})
	d.SetConfig(Config{Enabled: true, SMTP: &SMTPConfig{Host: "mail", Password: "********"}})
	if got := d.Config().SMTP.Password; got != "s3cret" {
		t.Fatalf("password = %q, want the retained secret", got)
	}
}

func TestRedactedHidesSecrets(t *testing.T) {
	c := Config{SMTP: &SMTPConfig{Password: "p"}, Ntfy: &NtfyConfig{Token: "tok"}}.Redacted()
	if c.SMTP.Password == "p" || c.Ntfy.Token == "tok" {
		t.Fatalf("secrets not redacted: %+v", c)
	}
}

func TestNtfySend(t *testing.T) {
	var title, prio string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		title = r.Header.Get("Title")
		prio = r.Header.Get("Priority")
	}))
	defer srv.Close()

	d := New(Config{Enabled: true, Ntfy: &NtfyConfig{BaseURL: srv.URL, Topic: "alerts"}})
	if err := d.send(d.Config(), "onaccess-detection", "hit"); err != nil {
		t.Fatal(err)
	}
	if title == "" || prio != "urgent" {
		t.Fatalf("title=%q prio=%q", title, prio)
	}
}
