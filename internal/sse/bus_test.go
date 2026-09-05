package sse

import "testing"

func TestPublishDelivers(t *testing.T) {
	b := NewBus()
	s1, c1 := b.Subscribe()
	s2, c2 := b.Subscribe()
	defer c1()
	defer c2()

	if b.SubscriberCount() != 2 {
		t.Fatalf("SubscriberCount = %d", b.SubscriberCount())
	}
	b.Publish(Event{Type: "x", Data: 1})

	for _, s := range []<-chan Event{s1, s2} {
		ev := <-s
		if ev.Type != "x" {
			t.Errorf("got %+v", ev)
		}
	}
}

func TestCancelRemovesSubscriber(t *testing.T) {
	b := NewBus()
	_, cancel := b.Subscribe()
	cancel()
	cancel() // idempotent
	if b.SubscriberCount() != 0 {
		t.Fatalf("SubscriberCount = %d after cancel", b.SubscriberCount())
	}
}

func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	b := NewBus()
	_, cancel := b.Subscribe() // never drained
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			b.Publish(Event{Type: "flood"})
		}
		close(done)
	}()
	<-done // must not deadlock
}
