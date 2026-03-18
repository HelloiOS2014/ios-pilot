package core

import (
	"context"
	"strings"
	"sync"

	"ios-pilot/internal/driver"
)

// RingBuffer is a fixed-capacity circular buffer for LogEntry values.
type RingBuffer struct {
	entries []driver.LogEntry
	size    int
	head    int // points to the slot where the next write goes
	count   int
	mu      sync.RWMutex
}

// NewRingBuffer creates a RingBuffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		entries: make([]driver.LogEntry, capacity),
		size:    capacity,
	}
}

// Add inserts an entry into the ring buffer, overwriting the oldest entry when full.
func (rb *RingBuffer) Add(entry driver.LogEntry) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.entries[rb.head] = entry
	rb.head = (rb.head + 1) % rb.size
	if rb.count < rb.size {
		rb.count++
	}
}

// GetLast returns the n most recent entries in chronological order.
// If n <= 0 or n >= count, all entries are returned.
func (rb *RingBuffer) GetLast(n int) []driver.LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return []driver.LogEntry{}
	}

	if n <= 0 || n >= rb.count {
		n = rb.count
	}

	result := make([]driver.LogEntry, n)
	// The oldest stored entry starts at head (when buffer is full) or at 0 (when not full).
	// We want the last n entries: start index from tail.
	start := (rb.head - n + rb.size*2) % rb.size // ensure positive modulo
	// Re-calculate properly for when count < size.
	// Logical index of the oldest entry:
	var oldest int
	if rb.count < rb.size {
		oldest = 0
	} else {
		oldest = rb.head
	}

	// We want entries at logical indices [count-n .. count-1]
	skip := rb.count - n
	for i := 0; i < n; i++ {
		physIdx := (oldest + skip + i) % rb.size
		result[i] = rb.entries[physIdx]
	}
	_ = start // suppress unused warning
	return result
}

// GetAll returns all entries in chronological order.
func (rb *RingBuffer) GetAll() []driver.LogEntry {
	return rb.GetLast(0)
}

// LogFilter defines optional predicates to narrow log results.
type LogFilter struct {
	BundleID string // filter by process/bundle
	Level    string // exact match on Level field
	Search   string // substring search in Message or Raw
}

// LogManager collects syslog entries into a ring buffer and
// fans them out to subscribers.
type LogManager struct {
	buffer      *RingBuffer
	syslogDrv   driver.SyslogDriver
	crashDrv    driver.CrashDriver
	udid        string
	cancel      context.CancelFunc
	running     bool
	mu          sync.Mutex
	subscribers []chan driver.LogEntry
}

// NewLogManager constructs a LogManager backed by the supplied drivers.
func NewLogManager(sd driver.SyslogDriver, cd driver.CrashDriver, bufferSize int) *LogManager {
	return &LogManager{
		buffer:    NewRingBuffer(bufferSize),
		syslogDrv: sd,
		crashDrv:  cd,
	}
}

// Start begins streaming syslog entries from the device identified by udid
// into the internal ring buffer. It is safe to call Start again after Stop.
func (lm *LogManager) Start(udid string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if lm.running {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	lm.cancel = cancel
	lm.udid = udid
	lm.running = true

	ch := make(chan driver.LogEntry, 256)

	// Feed entries from driver into channel in a background goroutine.
	go func() {
		_ = lm.syslogDrv.StartSyslog(ctx, udid, ch)
	}()

	// Drain channel, store in buffer, fan-out to subscribers.
	go func() {
		for {
			select {
			case entry, ok := <-ch:
				if !ok {
					return
				}
				lm.buffer.Add(entry)
				lm.fanOut(entry)
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Stop cancels the syslog collection goroutine.
func (lm *LogManager) Stop() {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if !lm.running {
		return
	}
	lm.cancel()
	lm.running = false
}

// IsRunning reports whether log collection is active.
func (lm *LogManager) IsRunning() bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	return lm.running
}

// GetLogs returns up to n log entries that satisfy filter.
// If n <= 0, all buffered entries are considered.
func (lm *LogManager) GetLogs(n int, filter LogFilter) []driver.LogEntry {
	var all []driver.LogEntry
	if n > 0 {
		all = lm.buffer.GetLast(n)
	} else {
		all = lm.buffer.GetAll()
	}

	if filter.BundleID == "" && filter.Level == "" && filter.Search == "" {
		return all
	}

	result := make([]driver.LogEntry, 0, len(all))
	for _, e := range all {
		if filter.BundleID != "" && e.Process != filter.BundleID {
			continue
		}
		if filter.Level != "" && e.Level != filter.Level {
			continue
		}
		if filter.Search != "" {
			if !strings.Contains(e.Message, filter.Search) && !strings.Contains(e.Raw, filter.Search) {
				continue
			}
		}
		result = append(result, e)
	}
	return result
}

// Subscribe returns a channel that receives new log entries as they arrive and
// an unsubscribe function that removes the subscription and closes the channel.
func (lm *LogManager) Subscribe() (<-chan driver.LogEntry, func()) {
	ch := make(chan driver.LogEntry, 100)

	lm.mu.Lock()
	lm.subscribers = append(lm.subscribers, ch)
	lm.mu.Unlock()

	unsub := func() {
		lm.mu.Lock()
		defer lm.mu.Unlock()
		for i, sub := range lm.subscribers {
			if sub == ch {
				lm.subscribers = append(lm.subscribers[:i], lm.subscribers[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, unsub
}

// fanOut delivers entry to all current subscribers (non-blocking).
// Must be called without lm.mu held.
func (lm *LogManager) fanOut(entry driver.LogEntry) {
	lm.mu.Lock()
	subs := make([]chan driver.LogEntry, len(lm.subscribers))
	copy(subs, lm.subscribers)
	lm.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
			// Drop if subscriber is slow.
		}
	}
}

// ListCrashes delegates to the crash driver.
func (lm *LogManager) ListCrashes() ([]driver.CrashReport, error) {
	return lm.crashDrv.ListCrashes(lm.udid)
}

// GetCrash retrieves a single crash report by id.
func (lm *LogManager) GetCrash(id string) (*driver.CrashReport, error) {
	return lm.crashDrv.GetCrash(lm.udid, id)
}
