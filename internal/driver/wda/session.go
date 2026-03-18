package wda

import (
	"fmt"
	"sync"
)

// SessionManager manages a single WDA session, creating it on demand
// and caching the session ID for subsequent calls.
type SessionManager struct {
	wdaURL    string
	sessionID string
	client    *WDAClient
	mu        sync.Mutex
}

// NewSessionManager creates a SessionManager for the given WDA base URL.
func NewSessionManager(wdaURL string) *SessionManager {
	return &SessionManager{
		wdaURL: wdaURL,
		client: NewWDAClient(),
	}
}

// EnsureSession returns the current session ID, creating a new session if
// one does not already exist.
func (sm *SessionManager) EnsureSession() (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.sessionID != "" {
		return sm.sessionID, nil
	}

	id, err := sm.client.CreateSession(sm.wdaURL)
	if err != nil {
		return "", fmt.Errorf("ensure session: %w", err)
	}
	sm.sessionID = id
	return sm.sessionID, nil
}

// IsHealthy returns true when WDA reports it is ready.
func (sm *SessionManager) IsHealthy() bool {
	ready, err := sm.client.Status(sm.wdaURL)
	if err != nil {
		return false
	}
	return ready
}

// Destroy terminates the active session (if any) and clears the cached ID.
func (sm *SessionManager) Destroy() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.sessionID == "" {
		return nil
	}

	if err := sm.client.DeleteSession(sm.wdaURL, sm.sessionID); err != nil {
		return fmt.Errorf("destroy session: %w", err)
	}
	sm.sessionID = ""
	return nil
}

// SessionID returns the cached session ID, or "" if no session has been
// created yet.
func (sm *SessionManager) SessionID() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.sessionID
}
