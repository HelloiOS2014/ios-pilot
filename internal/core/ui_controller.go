package core

import (
	"ios-pilot/internal/driver"
	"ios-pilot/internal/protocol"
)

// UiController dispatches UI interaction commands to WebDriverAgent.
// All methods guard against disconnected or degraded-mode device state.
type UiController struct {
	wdaDriver     driver.WDADriver
	deviceManager *DeviceManager
}

// NewUiController constructs a UiController.
func NewUiController(wd driver.WDADriver, dm *DeviceManager) *UiController {
	return &UiController{
		wdaDriver:     wd,
		deviceManager: dm,
	}
}

// requireFullMode ensures a device is connected and operating in full mode.
// Returns an error suitable for the caller if either precondition is not met.
func (uc *UiController) requireFullMode() error {
	if !uc.deviceManager.IsConnected() {
		return protocol.ErrDeviceNotConnected.ToError(nil)
	}
	if uc.deviceManager.Mode() == "degraded" {
		return protocol.ErrWDAUnavailable.ToError(nil)
	}
	return nil
}

// Tap sends a tap gesture to the given (x, y) screen coordinate.
func (uc *UiController) Tap(x, y int) error {
	if err := uc.requireFullMode(); err != nil {
		return err
	}
	return uc.wdaDriver.Tap(
		uc.deviceManager.WDAURL(),
		uc.deviceManager.WDASessionID(),
		x, y,
	)
}

// Swipe performs a swipe from (x1, y1) to (x2, y2).
func (uc *UiController) Swipe(x1, y1, x2, y2 int) error {
	if err := uc.requireFullMode(); err != nil {
		return err
	}
	return uc.wdaDriver.Swipe(
		uc.deviceManager.WDAURL(),
		uc.deviceManager.WDASessionID(),
		x1, y1, x2, y2,
	)
}

// Input types text into the currently focused element.
func (uc *UiController) Input(text string) error {
	if err := uc.requireFullMode(); err != nil {
		return err
	}
	return uc.wdaDriver.InputText(
		uc.deviceManager.WDAURL(),
		uc.deviceManager.WDASessionID(),
		text,
	)
}

// Press presses a hardware or virtual button (e.g. "home", "volumeUp").
func (uc *UiController) Press(key string) error {
	if !uc.deviceManager.IsConnected() {
		return protocol.ErrDeviceNotConnected.ToError(nil)
	}
	// Press can operate in degraded mode for hardware buttons; WDA required otherwise.
	if uc.deviceManager.Mode() == "degraded" {
		return protocol.ErrWDAUnavailable.ToError(nil)
	}
	return uc.wdaDriver.PressButton(
		uc.deviceManager.WDAURL(),
		uc.deviceManager.WDASessionID(),
		key,
	)
}
