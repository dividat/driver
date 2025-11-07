package flex

/* Connects to Senso Flex devices through a serial connection and combines serial data into measurement sets.

This helps establish an indirect WebSocket connection to receive a stream of samples from the device.

The functionality of this module is as follows:

- While connected, scan for serial devices that look like a potential Flex device
- Connect to suitable serial devices and start polling for measurements
- Minimally parse incoming data to determine start and end of a measurement
- Send each complete measurement set to client as a binary package

*/

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cskr/pubsub"
	"github.com/sirupsen/logrus"
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"

	"github.com/dividat/driver/src/dividat-driver/util"
	"github.com/dividat/driver/src/dividat-driver/util/websocket"
)

// how often to look for Flex devices while there are clients and no devices are
// connected
const backgroundScanIntervalSeconds = 2

// pubsub topic names, must be unique
const brokerTopicRx = "flex-rx"
const brokerTopicTx = "flex-tx"
const brokerTopicRxBroadcast = "flex-rx-broadcast"

// Handle for managing Flex
type Handle struct {
	websocket.Handle
}

type DeviceBackend struct {
	ctx context.Context
	log *logrus.Entry

	currentDevice *websocket.UsbDeviceInfo

	broker *pubsub.PubSub

	cancelCurrentConnection context.CancelFunc
	connectionChangeMutex   *sync.Mutex

	backgroundScanCancel context.CancelFunc

	subscriberCount int
}

// New returns an initialized handler
func New(ctx context.Context, log *logrus.Entry) *Handle {
	backend := DeviceBackend{
		ctx: ctx,
		log: log,

		broker: pubsub.New(32),

		connectionChangeMutex: &sync.Mutex{},

		subscriberCount: 0,
	}

	websocketHandle := websocket.Handle{
		DeviceBackend:     &backend,
		Broker:            backend.broker,
		BrokerRx:          brokerTopicRx,
		BrokerTx:          brokerTopicTx,
		BrokerRxBroadcast: util.PointerTo(brokerTopicRxBroadcast),
		Log:               log,
	}

	handle := Handle{Handle: websocketHandle}

	// Clean up
	go func() {
		<-ctx.Done()
		backend.broker.Shutdown()
	}()

	return &handle
}

func (backend *DeviceBackend) broadcastMessage(msg websocket.Message) {
	backend.broker.TryPub(msg, brokerTopicRxBroadcast)
}

func (backend *DeviceBackend) broadcastStatusUpdate() {
	status := backend.GetStatus()
	backend.broadcastMessage(websocket.Message{Status: &status})
}

// connect to a "validated" device
func (backend *DeviceBackend) connectInternal(device websocket.UsbDeviceInfo) error {
	// Only allow one connection change at a time
	backend.connectionChangeMutex.Lock()
	defer backend.connectionChangeMutex.Unlock()

	// disconnect current connection first
	backend.Disconnect()

	backend.log.WithField("path", device.Path).Info("Attempting to connect with device.")

	ctx, cancel := context.WithCancel(backend.ctx)

	onReceive := func(data []byte) {
		backend.broker.TryPub(data, brokerTopicRx)
	}

	backend.log.WithField("path", device.Path).Info("Attempting to open serial port.")
	port, err := backend.openSerial(device.Path)
	if err != nil {
		backend.log.WithField("path", device.Path).WithField("error", err).Info("Failed to open connection to serial port.")
		return err
	}
	backend.currentDevice = &device

	backend.cancelCurrentConnection = func() {
		backend.log.Debug("Cancelling the current connection.")
		cancel()
		backend.currentDevice = nil
		backend.cancelCurrentConnection = nil
		backend.broadcastStatusUpdate()
	}
	backend.broadcastStatusUpdate()

	// TODO: seems to work, but look at ctx/cancel a bit more carefully, at
	// a minimum it feels like some parts are redundant
	go connectSerial(ctx, backend.cancelCurrentConnection, backend.log, port, backend.broker.Sub(brokerTopicTx), onReceive)
	return nil
}

func (backend *DeviceBackend) connectToFirstIfNotConnected() {
	if backend.cancelCurrentConnection != nil {
		// already connected, nothing to do
		return
	}

	devices := backend.listMatchingSerialDevices()

	// try devices until the first success
	for _, device := range devices {
		err := backend.connectInternal(device)
		if err == nil {
			return
		}
	}
}

func (backend *DeviceBackend) disableAutoConnect() {
	if backend.backgroundScanCancel != nil {
		backend.backgroundScanCancel()
		backend.backgroundScanCancel = nil
	}
}

func (backend *DeviceBackend) enableAutoConnect() {
	if backend.backgroundScanCancel == nil {
		ctx, cancel := context.WithCancel(backend.ctx)
		go backend.backgroundScan(ctx)
		backend.backgroundScanCancel = cancel
	}
}

func (backend *DeviceBackend) backgroundScan(ctx context.Context) {
	// TODO: replace with udev on Linux at least?
	ticker := time.NewTicker(backgroundScanIntervalSeconds * time.Second)
	defer func() {
		backend.log.Info("Stopping background scan and auto-connect")
		ticker.Stop()
	}()

	backend.log.Info("Background scan and auto-connect started")

	for {
		select {
		case <-ticker.C:
			backend.connectToFirstIfNotConnected()

		case <-ctx.Done():
			return
		}
	}

}

func (backend *DeviceBackend) RegisterSubscriber(req *http.Request) {
	backend.subscriberCount++

	// disables the background scan-and-autoconnect
	// TODO: does last-client-wins logic make sense?
	if req.Header.Get("manual-connect") == "1" {
		backend.disableAutoConnect()
	} else {
		// backwards compat: if header is not set, auto-connect
		backend.connectToFirstIfNotConnected()
		backend.enableAutoConnect()
	}
}

// Deregister subscribers and disconnect when none left
func (backend *DeviceBackend) DeregisterSubscriber() {
	backend.subscriberCount--

	if backend.subscriberCount == 0 && backend.cancelCurrentConnection != nil {
		backend.disableAutoConnect()
		backend.Disconnect()
	}
}

// Check whether a port looks like a potential Flex device.
//
// Vendor IDs:
//
//	16C0 - Van Ooijen Technische Informatica (Teensy)
func isFlexLike(port enumerator.PortDetails) bool {
	vendorId := strings.ToUpper(port.VID)

	return vendorId == "16C0"
}

func (backend *DeviceBackend) listMatchingSerialDevices() []websocket.UsbDeviceInfo {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		backend.log.WithField("error", err).Info("Could not list serial devices.")
		return nil
	}
	var matching []websocket.UsbDeviceInfo
	for _, port := range ports {
		backend.log.WithField("name", port.Name).WithField("vendor", port.VID).Debug("Considering serial port.")

		if isFlexLike(*port) {
			device, err := portDetailsToDeviceInfo(*port)
			if err != nil {
				backend.log.WithField("port", port).Error("Failed to convert serial port details to device info!")
			} else {
				backend.log.WithField("name", port.Name).Debug("Serial port matches a Flex device.")
				matching = append(matching, *device)
			}
		}
	}
	return matching
}

func portDetailsToDeviceInfo(port enumerator.PortDetails) (*websocket.UsbDeviceInfo, error) {
	idVendor, err := strconv.ParseUint(port.VID, 16, 16) // hex, uint16
	if err != nil {
		return nil, err
	}
	idProduct, err := strconv.ParseUint(port.PID, 16, 16) // hex, uint16
	if err != nil {
		return nil, err
	}

	deviceInfo := websocket.UsbDeviceInfo{
		Path:         port.Name,
		IdVendor:     uint16(idVendor),
		IdProduct:    uint16(idProduct),
		SerialNumber: port.SerialNumber,
		Manufacturer: port.Manufacturer,
		Product:      port.Product,
	}
	return &deviceInfo, nil
}

func (backend *DeviceBackend) GetStatus() websocket.Status {
	status := websocket.Status{}

	if backend.currentDevice != nil {
		status.Address = &backend.currentDevice.Path
		status.DeviceInfo = &websocket.DeviceInfo{UsbDeviceInfo: backend.currentDevice}
	}
	return status
}

// NOTE: The remaining Driver commands are not currently used in Play for Flex

func (backend *DeviceBackend) lookupDeviceInfo(portName string) *websocket.UsbDeviceInfo {
	devices := backend.listMatchingSerialDevices()
	for _, device := range devices {
		if device.Path == portName {
			return &device
		}
	}
	return nil
}

// Connect to device using only the address (path, e.g. "/dev/ttyACM0")
// Currently not used in Play
func (backend *DeviceBackend) Connect(address string) {
	port := backend.lookupDeviceInfo(address)
	if port == nil {
		backend.log.WithField("address", address).Error("Could not look up device, aborting Connect.")
		return
	} else {
		backend.connectInternal(*port)
	}

}

// Currently not used in Play
func (backend *DeviceBackend) Disconnect() {
	if backend.cancelCurrentConnection != nil {
		backend.cancelCurrentConnection()
	}
}

// Currently not used in Play
func (backend *DeviceBackend) Discover(duration int, ctx context.Context) chan websocket.DeviceInfo {
	matching := backend.listMatchingSerialDevices()
	devices := make(chan websocket.DeviceInfo)

	go func(usbDevices []websocket.UsbDeviceInfo) {
		for _, usbDevice := range usbDevices {
			// Terminate if we have been cancelled
			if ctx.Err() != nil {
				break
			}

			device := websocket.DeviceInfo{UsbDeviceInfo: &usbDevice}

			devices <- device
		}

		close(devices)
	}(matching)
	return devices
}

// not supported
func (backend *DeviceBackend) IsUpdatingFirmware() bool {
	return false
}

// not supported
func (backend *DeviceBackend) ProcessFirmwareUpdateRequest(command websocket.UpdateFirmware, send websocket.SendMsg) {
	// noop
	return
}

// Serial communication

type ReaderState int

const (
	WAITING_FOR_HEADER ReaderState = iota
	HEADER_START
	HEADER_READ_LENGTH_MSB
	HEADER_READ_LENGTH_LSB
	WAITING_FOR_BODY
	BODY_START
	BODY_READ_SAMPLE
	UNEXPECTED_BYTE
)

const (
	HEADER_START_MARKER = 'N'
	BODY_START_MARKER   = 'P'
)

const (
	// row, column and pressure value, one uint8 each
	BYTES_PER_SAMPLE_8BIT = 3

	// same as above, but pressure value is 2 bytes (uint16), big-endian
	// Note: Sensing Tex docs state value max is 2^12-1 (hence "12bit"),
	// but in practice they seem to send values up to ~9000, so more like
	// "14 bit".
	BYTES_PER_SAMPLE_12BIT = 4
)

var BITDEPTH_8_CMD = []byte{'U', 'L', '\n'}

var BITDEPTH_12_CMD = []byte{'U', 'M', '\n'}

func isBitdepthCommand(cmd []byte) bool {
	return bytes.Equal(cmd, BITDEPTH_8_CMD) || bytes.Equal(cmd, BITDEPTH_12_CMD)
}

func bitdepthCommandToBytesPerSample(cmd []byte) int {
	if bytes.Equal(cmd, BITDEPTH_8_CMD) {
		return BYTES_PER_SAMPLE_8BIT
	} else {
		return BYTES_PER_SAMPLE_12BIT
	}
}

func (backend *DeviceBackend) openSerial(serialName string) (serial.Port, error) {
	mode := &serial.Mode{
		BaudRate: 115200,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(serialName, mode)
	if err != nil {
		return nil, err
	}

	return port, nil
}

// Read from the serial port and pipe its signal into the callback, summarizing
// package units into a buffer. Forward commands from client.
func connectSerial(ctx context.Context, cancel context.CancelFunc, logger *logrus.Entry, port serial.Port, tx chan interface{}, onReceive func([]byte)) {
	readerCtx, readerCtxCancel := context.WithCancel(ctx)

	defer func() {
		logger.Info("Disconnecting from serial port.")
		port.Close()
		readerCtxCancel()
		cancel()
	}()

	port.ResetInputBuffer() // flush any unread data buffered by the OS

	// For backwards compatibility, we set 8bit depth by default
	_, err := port.Write(BITDEPTH_8_CMD)
	if err != nil {
		logger.WithField("error", err).Info("Failed to set bitdepth of 8.")
		return
	}
	configuredBytesPerSample := BYTES_PER_SAMPLE_8BIT

	// Channel to receive ack that reader is done
	readerDoneChan := make(chan struct{})

	// Start the initial reader goroutine
	go readFromPort(readerCtx, logger, port, configuredBytesPerSample, onReceive, readerDoneChan)

	// Forward WebSocket commands to device
	for {
		select {
		case <-ctx.Done():
			return

		case <-readerDoneChan:
			return

		case i := <-tx:
			data, _ := i.([]byte)

			if isBitdepthCommand(data) {
				newBytesPerSample := bitdepthCommandToBytesPerSample(data)

				// If bytes per sample has changed, we need to restart the reader
				if newBytesPerSample != configuredBytesPerSample {
					logger.WithFields(logrus.Fields{
						"old": configuredBytesPerSample,
						"new": newBytesPerSample,
					}).Info("Bytes per sample changed, will restart reader")

					logger.Debug("Sending stop to reader and waiting for ack")
					readerCtxCancel()
					<-readerDoneChan
					logger.Debug("Ack received, reader stopped")

					_, err = port.Write(data)
					if err != nil {
						logger.WithField("error", err).Info("Failed to write new bitdepth.")
						return
					}

					port.ResetInputBuffer() // flush any data that was not yet read

					configuredBytesPerSample = newBytesPerSample

					readerCtx, readerCtxCancel = context.WithCancel(ctx)
					readerDoneChan = make(chan struct{})

					// Start a new reader goroutine with updated bytesPerSample
					go readFromPort(readerCtx, logger, port, configuredBytesPerSample, onReceive, readerDoneChan)
				}
			} else {
				// For non-bitdepth commands, just forward them
				_, err = port.Write(data)
				if err != nil {
					logger.WithField("error", err).Info("Failed to write command to serial port.")
					return
				}
				logger.WithField("bytes", data).Debug("Wrote binary command to serial out: " + string(data))
			}
		}
	}
}

// Infinite loop for requesting and reading serial data.
// Stops (returns) upon any error or ctx cancel.
func readFromPort(
	ctx context.Context,
	logger *logrus.Entry,
	port serial.Port,
	bytesPerSample int,
	onReceive func([]byte),
	doneChan chan<- struct{},
) {
	defer func() {
		// Signal that the reader has completed
		close(doneChan)
	}()

	reader := bufio.NewReader(port)
	state := WAITING_FOR_HEADER
	var samplesLeftInSet int
	var bytesLeftInSample int

	// Note: for Flex v4 this command seems to cause the firmware to push
	// data as fast as we can consume it, whereas for Flex v5 it merely
	// requests a single frame.
	START_MEASUREMENT_CMD := []byte{'S', '\n'}
	_, err := port.Write(START_MEASUREMENT_CMD)
	if err != nil {
		logger.WithField("error", err).Info("Failed to write start message to serial port.")
		return
	}

	// Start signal acquisition
	var buff []byte
	for {
		// Terminate if we were cancelled
		if ctx.Err() != nil {
			logger.Debug("Stopping reader: context cancelled")
			return
		}

		input, err := reader.ReadByte()
		if err != nil {
			logger.WithField("err", err).Error("Error reading from serial port")
			return
		}

		var length_msb byte

		// Finite State Machine for parsing byte stream
		switch {
		case state == WAITING_FOR_HEADER && input == HEADER_START_MARKER:
			state = HEADER_START
		case state == HEADER_START && input == '\n':
			state = HEADER_READ_LENGTH_MSB
		case state == HEADER_READ_LENGTH_MSB:
			// The number of measurements in each set is given as two
			// consecutive bytes (big-endian).
			length_msb = input
			state = HEADER_READ_LENGTH_LSB
		case state == HEADER_READ_LENGTH_LSB:
			length_lsb := input
			samplesLeftInSet = int(binary.BigEndian.Uint16([]byte{length_msb, length_lsb}))
			state = WAITING_FOR_BODY
		case state == WAITING_FOR_BODY && input == BODY_START_MARKER:
			state = BODY_START
		case state == BODY_START && input == '\n':
			state = BODY_READ_SAMPLE
			buff = []byte{}
			bytesLeftInSample = bytesPerSample
		case state == BODY_READ_SAMPLE:
			buff = append(buff, input)
			bytesLeftInSample = bytesLeftInSample - 1

			if bytesLeftInSample <= 0 {
				samplesLeftInSet = samplesLeftInSet - 1

				if samplesLeftInSet <= 0 {
					// Finish and send set
					onReceive(buff)

					// Get ready for next set and request it
					state = WAITING_FOR_HEADER
					// Optional for Flex v4, mandatory for v5
					_, err = port.Write(START_MEASUREMENT_CMD)
					if err != nil {
						logger.WithField("error", err).Info("Failed to write poll message to serial port.")
						return
					}
				} else {
					// Start next point
					bytesLeftInSample = bytesPerSample
				}
			}
		case state == UNEXPECTED_BYTE && input == HEADER_START_MARKER:
			// Recover from error state when a new header is seen
			state = HEADER_START
		default:
			state = UNEXPECTED_BYTE
		}
	}
}
