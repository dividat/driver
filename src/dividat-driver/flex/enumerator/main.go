package enumerator

import (
	"context"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	serialenum "go.bug.st/serial/enumerator"

	"github.com/dividat/driver/src/dividat-driver/flex/enumerator/mockdev"
	"github.com/dividat/driver/src/dividat-driver/protocol"
	"github.com/dividat/driver/src/dividat-driver/util"
)

type DeviceFamily int

const (
	DeviceFamilyPassthru DeviceFamily = iota
	DeviceFamilySensingTex
	DeviceFamilySensitronics
)

type DeviceEnumerator struct {
	ctx                context.Context
	log                *logrus.Entry
	mockDeviceRegistry *mockdev.MockDeviceRegistry
}

func New(ctx context.Context, log *logrus.Entry, mockDeviceRegistry *mockdev.MockDeviceRegistry) *DeviceEnumerator {
	return &DeviceEnumerator{
		ctx:                ctx,
		log:                log,
		mockDeviceRegistry: mockDeviceRegistry,
	}
}

func (handle *DeviceEnumerator) getSerialPortList() ([]*serialenum.PortDetails, error) {
	realDevices, err := serialenum.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}

	mockDevices := handle.mockDeviceRegistry.ListMockDevices()

	allDevices := append(realDevices, mockDevices...)

	return allDevices, nil
}

// Check whether a port looks like a potential Flex device.
//
// Vendor IDs:
//
//	16C0 - Van Ooijen Technische Informatica (Teensy)
func isTeensyDevice(device protocol.UsbDeviceInfo) bool {
	return device.IdVendor == 0x16C0
}

func findMatchingDeviceFamily(device protocol.UsbDeviceInfo) *DeviceFamily {
	if !isTeensyDevice(device) {
		return nil
	}

	if strings.HasPrefix(device.Product, "PASSTHRU") {
		return util.PointerTo(DeviceFamilyPassthru)
	}

	if device.Manufacturer == "Teensyduino" {
		return util.PointerTo(DeviceFamilySensingTex)
	} else if device.Manufacturer == "Sensitronics" || device.Manufacturer == "Dividat" {
		return util.PointerTo(DeviceFamilySensitronics)
	}

	return nil
}

type MatchedDevice struct {
	Family DeviceFamily
	Info   protocol.UsbDeviceInfo
}

func (handle *DeviceEnumerator) ListMatchingDevices() []MatchedDevice {
	ports, err := handle.getSerialPortList()
	if err != nil {
		handle.log.WithField("error", err).Info("Could not list serial devices.")
		return nil
	}
	var matching []MatchedDevice
	for _, port := range ports {
		handle.log.WithField("name", port.Name).WithField("vendor", port.VID).Debug("Considering serial port.")

		device, err := portDetailsToDeviceInfo(*port)
		if err != nil {
			handle.log.WithField("port", port).WithField("err", err).Error("Failed to convert serial port details to device info!")
			continue
		}

		family := findMatchingDeviceFamily(*device)

		if family != nil {
			handle.log.WithField("name", port.Name).WithField("family", *family).Debug("Serial port matches a Flex device.")
			matchedDevice := MatchedDevice{Family: *family, Info: *device}
			matching = append(matching, matchedDevice)
		}
	}
	return matching
}

func portDetailsToDeviceInfo(port serialenum.PortDetails) (*protocol.UsbDeviceInfo, error) {
	idVendor, err := strconv.ParseUint(port.VID, 16, 16) // hex, uint16
	if err != nil {
		return nil, err
	}
	idProduct, err := strconv.ParseUint(port.PID, 16, 16) // hex, uint16
	if err != nil {
		return nil, err
	}
	bcdDevice, err := strconv.ParseUint(port.BcdDevice, 16, 16) // hex, uint16
	if err != nil {
		return nil, err
	}

	deviceInfo := protocol.UsbDeviceInfo{
		Path:         port.Name,
		IdVendor:     uint16(idVendor),
		IdProduct:    uint16(idProduct),
		BcdDevice:    uint16(bcdDevice),
		SerialNumber: port.SerialNumber,
		Manufacturer: port.Manufacturer,
		Product:      port.Product,
	}
	return &deviceInfo, nil
}
