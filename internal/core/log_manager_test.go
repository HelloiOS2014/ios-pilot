package core

import (
	"context"
	"testing"
	"time"

	"ios-pilot/internal/driver"
)

// ---- mock syslog driver ----

// mockSyslogDriver feeds a pre-loaded slice of entries into the channel.
type mockSyslogDriver struct {
	entries []driver.LogEntry
}

func (m *mockSyslogDriver) StartSyslog(ctx context.Context, _ string, ch chan<- driver.LogEntry) error {
	for _, e := range m.entries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- e:
		}
	}
	// Block until context is cancelled.
	<-ctx.Done()
	return ctx.Err()
}

// ---- mock crash driver ----

type mockCrashDriver struct {
	crashes  []driver.CrashReport
	listErr  error
	getReport *driver.CrashReport
	getErr   error
}

func (m *mockCrashDriver) ListCrashes(_ string) ([]driver.CrashReport, error) {
	return m.crashes, m.listErr
}

func (m *mockCrashDriver) GetCrash(_ string, _ string) (*driver.CrashReport, error) {
	return m.getReport, m.getErr
}

// ---- RingBuffer tests ----

func TestRingBuffer_AddAndGet(t *testing.T) {
	rb := NewRingBuffer(5)
	for i := 0; i < 7; i++ {
		rb.Add(driver.LogEntry{Message: string(rune('A' + i))})
	}
	// Buffer size 5; last 5 entries should be C,D,E,F,G (indices 2-6)
	got := rb.GetLast(5)
	if len(got) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(got))
	}
	expected := []string{"C", "D", "E", "F", "G"}
	for i, e := range got {
		if e.Message != expected[i] {
			t.Errorf("[%d] expected %q, got %q", i, expected[i], e.Message)
		}
	}
}

func TestRingBuffer_GetLast(t *testing.T) {
	rb := NewRingBuffer(10)
	for i := 0; i < 7; i++ {
		rb.Add(driver.LogEntry{Message: string(rune('A' + i))})
	}
	// Request 3 — should return the last 3: E, F, G
	got := rb.GetLast(3)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	expected := []string{"E", "F", "G"}
	for i, e := range got {
		if e.Message != expected[i] {
			t.Errorf("[%d] expected %q, got %q", i, expected[i], e.Message)
		}
	}
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := NewRingBuffer(5)
	got := rb.GetLast(3)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(got))
	}
}

// ---- LogManager tests ----

// addEntriesToBuffer is a helper that adds entries directly to the ring buffer,
// bypassing the syslog driver (avoids goroutine complexity in tests).
func newLogManagerWithEntries(entries []driver.LogEntry) *LogManager {
	lm := NewLogManager(&mockSyslogDriver{}, &mockCrashDriver{}, 100)
	for _, e := range entries {
		lm.buffer.Add(e)
	}
	return lm
}

func TestLogManager_GetLogs_FilterByProcess(t *testing.T) {
	entries := []driver.LogEntry{
		{Process: "com.example.app", Level: "info", Message: "hello"},
		{Process: "com.other.app", Level: "info", Message: "world"},
		{Process: "com.example.app", Level: "error", Message: "oops"},
	}
	lm := newLogManagerWithEntries(entries)

	got := lm.GetLogs(0, LogFilter{BundleID: "com.example.app"})
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	for _, e := range got {
		if e.Process != "com.example.app" {
			t.Errorf("unexpected process: %q", e.Process)
		}
	}
}

func TestLogManager_GetLogs_FilterByLevel(t *testing.T) {
	entries := []driver.LogEntry{
		{Process: "app", Level: "info", Message: "msg1"},
		{Process: "app", Level: "error", Message: "msg2"},
		{Process: "app", Level: "info", Message: "msg3"},
	}
	lm := newLogManagerWithEntries(entries)

	got := lm.GetLogs(0, LogFilter{Level: "error"})
	if len(got) != 1 {
		t.Fatalf("expected 1 error entry, got %d", len(got))
	}
	if got[0].Message != "msg2" {
		t.Errorf("unexpected message: %q", got[0].Message)
	}
}

func TestLogManager_GetLogs_FilterBySearch(t *testing.T) {
	entries := []driver.LogEntry{
		{Process: "app", Message: "connection refused"},
		{Process: "app", Message: "all good"},
		{Process: "app", Raw: "raw connection timeout"},
	}
	lm := newLogManagerWithEntries(entries)

	got := lm.GetLogs(0, LogFilter{Search: "connection"})
	if len(got) != 2 {
		t.Fatalf("expected 2 matching entries, got %d", len(got))
	}
}

func TestLogManager_Subscribe(t *testing.T) {
	lm := NewLogManager(&mockSyslogDriver{}, &mockCrashDriver{}, 100)

	ch, unsub := lm.Subscribe()
	defer unsub()

	entry := driver.LogEntry{Message: "test-subscribe"}
	lm.fanOut(entry)

	select {
	case got := <-ch:
		if got.Message != "test-subscribe" {
			t.Errorf("expected message 'test-subscribe', got %q", got.Message)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for subscribed entry")
	}
}

func TestLogManager_Subscribe_Unsubscribe(t *testing.T) {
	lm := NewLogManager(&mockSyslogDriver{}, &mockCrashDriver{}, 100)

	ch, unsub := lm.Subscribe()

	// Unsubscribe before sending.
	unsub()

	// Channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after unsubscribe")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestLogManager_CrashDelegation(t *testing.T) {
	reports := []driver.CrashReport{
		{ID: "cr1", Name: "MyApp", Process: "com.example.myapp", Timestamp: "2024-01-01T00:00:00Z"},
	}
	cd := &mockCrashDriver{
		crashes:   reports,
		getReport: &reports[0],
	}
	lm := NewLogManager(&mockSyslogDriver{}, cd, 100)
	lm.udid = "device-1"

	list, err := lm.ListCrashes()
	if err != nil {
		t.Fatalf("ListCrashes: %v", err)
	}
	if len(list) != 1 || list[0].ID != "cr1" {
		t.Errorf("unexpected crash list: %v", list)
	}

	got, err := lm.GetCrash("cr1")
	if err != nil {
		t.Fatalf("GetCrash: %v", err)
	}
	if got.ID != "cr1" {
		t.Errorf("expected crash ID cr1, got %q", got.ID)
	}
}
