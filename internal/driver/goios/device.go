// Package goios provides driver implementations backed by the go-ios library.
package goios

import (
	"fmt"

	goios "github.com/danielpaulus/go-ios/ios"
	"ios-pilot/internal/driver"
)

// GoIosDeviceDriver implements driver.DeviceDriver using the go-ios library.
type GoIosDeviceDriver struct{}

// Compile-time interface check.
var _ driver.DeviceDriver = (*GoIosDeviceDriver)(nil)

// NewDeviceDriver creates a new GoIosDeviceDriver.
func NewDeviceDriver() *GoIosDeviceDriver {
	return &GoIosDeviceDriver{}
}

// ListDevices returns all currently connected iOS devices.
func (d *GoIosDeviceDriver) ListDevices() ([]driver.DeviceInfo, error) {
	list, err := goios.ListDevices()
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}

	infos := make([]driver.DeviceInfo, 0, len(list.DeviceList))
	for _, entry := range list.DeviceList {
		udid := entry.Properties.SerialNumber
		if udid == "" {
			continue
		}
		info, err := deviceInfoFromEntry(entry)
		if err != nil {
			// Return partial results with just the UDID if lockdown fails.
			infos = append(infos, driver.DeviceInfo{UDID: udid})
			continue
		}
		infos = append(infos, *info)
	}
	return infos, nil
}

// GetDevice returns device info for the device with the given UDID.
// If udid is empty, the first connected device is returned.
func (d *GoIosDeviceDriver) GetDevice(udid string) (*driver.DeviceInfo, error) {
	entry, err := goios.GetDevice(udid)
	if err != nil {
		return nil, fmt.Errorf("get device %q: %w", udid, err)
	}
	return deviceInfoFromEntry(entry)
}

// deviceInfoFromEntry reads device details from lockdown and maps them to driver.DeviceInfo.
func deviceInfoFromEntry(entry goios.DeviceEntry) (*driver.DeviceInfo, error) {
	udid := entry.Properties.SerialNumber

	resp, err := goios.GetValues(entry)
	if err != nil {
		return nil, fmt.Errorf("get lockdown values for %q: %w", udid, err)
	}

	return &driver.DeviceInfo{
		UDID:        udid,
		Name:        resp.Value.DeviceName,
		IOSVersion:  resp.Value.ProductVersion,
		ProductType: resp.Value.ProductType,
	}, nil
}
