package enumerator

import (
	"context"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	serialenum "go.bug.st/serial/enumerator"

	"github.com/dividat/driver/src/dividat-driver/protocol"
)

type DeviceEnumerator struct {
	ctx                   context.Context
	log                   *logrus.Entry
	testMode              bool
	registeredMockDevices map[MockDeviceId]*serialenum.PortDetails
}

func New(ctx context.Context, log *logrus.Entry, testMode bool) *DeviceEnumerator {
	return &DeviceEnumerator{
		ctx:                   ctx,
		log:                   log,
		testMode:              testMode,
		registeredMockDevices: make(map[MockDeviceId]*serialenum.PortDetails),
	}
}

func (handle *DeviceEnumerator) getSerialPortList() ([]*serialenum.PortDetails, error) {
	// run even in testMode for a pseudo-test that enumeration works at all
	ports, err := serialenum.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}
	if handle.testMode {
		mockDevices := handle.listMockDevices()
		// in testMode, return ONLY the test devices to ensure tests work
		// consistently regardless of whether an actual Flex device is plugged in
		handle.log.WithField("n", len(mockDevices)).Debug("Returning mock devices")
		return mockDevices, nil
	} else {
		return ports, nil
	}
}

func (handle *DeviceEnumerator) ListMatchingDevices() []protocol.UsbDeviceInfo {
	ports, err := handle.getSerialPortList()
	if err != nil {
		handle.log.WithField("error", err).Info("Could not list serial devices.")
		return nil
	}
	var matching []protocol.UsbDeviceInfo
	for _, port := range ports {
		handle.log.WithField("name", port.Name).WithField("vendor", port.VID).Debug("Considering serial port.")

		if isFlexLike(*port) {
			device, err := portDetailsToDeviceInfo(*port)
			if err != nil {
				handle.log.WithField("port", port).WithField("err", err).Error("Failed to convert serial port details to device info!")
			} else {
				handle.log.WithField("name", port.Name).Debug("Serial port matches a Flex device.")
				matching = append(matching, *device)
			}
		}
	}
	return matching
}

// Check whether a port looks like a potential Flex device.
//
// Vendor IDs:
//
//	16C0 - Van Ooijen Technische Informatica (Teensy)
func isFlexLike(port serialenum.PortDetails) bool {
	vendorId := strings.ToUpper(port.VID)

	return vendorId == "16C0"
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
