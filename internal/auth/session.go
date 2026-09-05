package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// SessionIdleTimeout is how long a session survives without being used.
const SessionIdleTimeout = 12 * time.Hour

type sessionEntry struct {
	created  time.Time
	lastSeen time.Time
}

// SessionManager is an in-memory session store. Sessions do not survive a
// restart, which is acceptable for a single-admin tool.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]sessionEntry
	now      func() time.Time
}

// NewSessionManager returns an empty SessionManager.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]sessionEntry),
		now:      time.Now,
	}
}

// Create registers a new session and returns its opaque id.
func (m *SessionManager) Create() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(b)
	t := m.now()

	m.mu.Lock()
	m.sessions[id] = sessionEntry{created: t, lastSeen: t}
	m.mu.Unlock()
	return id, nil
}

// Valid reports whether id names a live session, refreshing its idle timer.
func (m *SessionManager) Valid(id string) bool {
	if id == "" {
		return false
	}
	t := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.sessions[id]
	if !ok {
		return false
	}
	if t.Sub(e.lastSeen) > SessionIdleTimeout {
		delete(m.sessions, id)
		return false
	}
	e.lastSeen = t
	m.sessions[id] = e
	return true
}

// Destroy removes a single session.
func (m *SessionManager) Destroy(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// DestroyAll clears every session. Call this after the admin password changes.
func (m *SessionManager) DestroyAll() {
	m.mu.Lock()
	m.sessions = make(map[string]sessionEntry)
	m.mu.Unlock()
}

// Count returns the number of live sessions (used in tests).
func (m *SessionManager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}
