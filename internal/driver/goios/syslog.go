package goios

import (
	"context"
	"fmt"
	"io"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/syslog"
	"ios-pilot/internal/driver"
)

// GoIosSyslogDriver implements driver.SyslogDriver using go-ios.
type GoIosSyslogDriver struct{}

// Compile-time interface check.
var _ driver.SyslogDriver = (*GoIosSyslogDriver)(nil)

// NewSyslogDriver creates a new GoIosSyslogDriver.
func NewSyslogDriver() *GoIosSyslogDriver {
	return &GoIosSyslogDriver{}
}

// StartSyslog streams syslog messages from the device into ch.
// It blocks until ctx is cancelled or an unrecoverable error occurs.
// The channel ch is not closed by this function; callers should close it if needed.
func (s *GoIosSyslogDriver) StartSyslog(ctx context.Context, udid string, ch chan<- driver.LogEntry) error {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return fmt.Errorf("syslog: get device: %w", err)
	}

	conn, err := syslog.New(entry)
	if err != nil {
		return fmt.Errorf("syslog: connect: %w", err)
	}
	defer conn.Close()

	// Build the log parser provided by go-ios.
	parse := syslog.Parser()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		raw, err := conn.ReadLogMessage()
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("syslog: read: %w", err)
		}

		entry := driver.LogEntry{Raw: raw}
		if parsed, parseErr := parse(raw); parseErr == nil {
			entry.Timestamp = parsed.Timestamp
			entry.Process = parsed.Process
			entry.PID = parsed.PID
			entry.Level = parsed.Level
			entry.Message = parsed.Message
		}

		select {
		case ch <- entry:
		case <-ctx.Done():
			return nil
		}
	}
}
