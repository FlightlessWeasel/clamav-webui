package clamav

import (
	"bytes"
	"strings"
	"testing"
)

func TestLimitedWriterCaps(t *testing.T) {
	var buf bytes.Buffer
	w := &limitedWriter{buf: &buf, n: 10}

	n, err := w.Write([]byte("hello"))
	if n != 5 || err != nil {
		t.Fatalf("Write 1 = %d, %v", n, err)
	}
	// Second write straddles the cap: 5 more accepted, rest discarded, but the
	// caller is still told everything was written so the command is not killed.
	n, err = w.Write([]byte("world, and then some"))
	if n != 20 || err != nil {
		t.Fatalf("Write 2 = %d, %v", n, err)
	}
	if got := buf.String(); got != "helloworld" {
		t.Errorf("buffered %q, want %q", got, "helloworld")
	}

	n, _ = w.Write([]byte("more"))
	if n != 4 || buf.Len() != 10 {
		t.Errorf("post-cap write changed buffer: len=%d", buf.Len())
	}
}

func TestLimitedWriterUnlimitedWhenNZero(t *testing.T) {
	var buf bytes.Buffer
	w := &limitedWriter{buf: &buf, n: 0}
	w.Write([]byte(strings.Repeat("x", 1000)))
	if buf.Len() != 0 {
		t.Errorf("n=0 should discard everything, got %d", buf.Len())
	}
}
