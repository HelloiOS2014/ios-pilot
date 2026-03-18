package goios

import (
	"fmt"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/instruments"
	"ios-pilot/internal/driver"
)

// GoIosScreenshotDriver implements driver.ScreenshotDriver using go-ios instruments.
type GoIosScreenshotDriver struct{}

// Compile-time interface check.
var _ driver.ScreenshotDriver = (*GoIosScreenshotDriver)(nil)

// NewScreenshotDriver creates a new GoIosScreenshotDriver.
func NewScreenshotDriver() *GoIosScreenshotDriver {
	return &GoIosScreenshotDriver{}
}

// TakeScreenshot captures the current screen of the device and returns PNG bytes.
func (s *GoIosScreenshotDriver) TakeScreenshot(udid string) ([]byte, error) {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return nil, fmt.Errorf("screenshot: get device: %w", err)
	}

	svc, err := instruments.NewScreenshotService(entry)
	if err != nil {
		return nil, fmt.Errorf("screenshot: connect to screenshot service: %w", err)
	}
	defer svc.Close()

	data, err := svc.TakeScreenshot()
	if err != nil {
		return nil, fmt.Errorf("screenshot: take screenshot: %w", err)
	}
	return data, nil
}
