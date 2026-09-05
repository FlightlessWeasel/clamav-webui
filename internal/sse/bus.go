// Package sse is a tiny in-process publish/subscribe bus whose events are
// streamed to browsers over Server-Sent Events.
package sse

import "sync"

// Event is one message. Type becomes the SSE "event:" field; Data is JSON-
// encoded into the "data:" field by the HTTP layer.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Bus fans out events to all current subscribers. A slow subscriber drops
// events rather than blocking publishers.
type Bus struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

// NewBus returns an empty Bus.
func NewBus() *Bus {
	return &Bus{subs: make(map[chan Event]struct{})}
}

// Subscribe returns a channel of events and a cancel func that closes it and
// removes the subscription. The channel is buffered; when full, Publish skips it.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, ch)
			b.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

// Publish delivers ev to every subscriber that has room in its buffer.
func (b *Bus) Publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// SubscriberCount reports how many subscribers are attached (used in tests).
func (b *Bus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}
