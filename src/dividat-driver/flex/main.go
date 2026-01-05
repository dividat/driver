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
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/cskr/pubsub"
	gorilla "github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"go.bug.st/serial"

	"github.com/dividat/driver/src/dividat-driver/flex/enumerator"
	"github.com/dividat/driver/src/dividat-driver/flex/passthru"
	"github.com/dividat/driver/src/dividat-driver/flex/sensingtex"
	"github.com/dividat/driver/src/dividat-driver/flex/sensitronics"
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

	enumerator *enumerator.DeviceEnumerator

	broker *pubsub.PubSub

	cancelCurrentConnection context.CancelFunc
	connectionChangeMutex   *sync.Mutex

	backgroundScanCancel context.CancelFunc

	subscriberCount int
}

// New returns an initialized handler
func New(ctx context.Context, log *logrus.Entry, enumerator *enumerator.DeviceEnumerator) *Handle {
	backend := DeviceBackend{
		ctx: ctx,
		log: log,

		enumerator: enumerator,

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
	broadcast := websocket.Broadcast{Message: msg}
	backend.broker.TryPub(broadcast, brokerTopicRxBroadcast)
}

func (backend *DeviceBackend) broadcastStatusUpdate() {
	status := backend.GetStatus()
	backend.broadcastMessage(websocket.Message{Status: &status})
}

type SerialReader interface {
	// Read from the serial port and pipe its signal into the callback, summarizing
	// package units into a buffer. Forward commands from client.
	ReadFromSerial(ctx context.Context, logger *logrus.Entry, port serial.Port, tx chan interface{}, onReceive func([]byte))
}

func deviceToReader(deviceInfo websocket.UsbDeviceInfo) SerialReader {
	if deviceInfo.Manufacturer == "Teensyduino" {
		return &sensingtex.SensingTexReader{}
	} else if deviceInfo.Manufacturer == "Sensitronics" {
		return &sensitronics.SensitronicsReader{}
	} else if deviceInfo.Product == "PASSTHRU" {
		return &passthru.PassthruReader{}
	}
	return nil
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
	reader := deviceToReader(device)
	// should not happen
	if reader == nil {
		backend.log.WithField("device", device).Error("Could not find reader for device!")
		port.Close()
		return err
	}

	backend.currentDevice = &device

	// TODO: replace this with context.AfterFunc
	backend.cancelCurrentConnection = func() {
		backend.log.Debug("Cancelling the current connection.")
		cancel()
		port.Close()
		backend.currentDevice = nil
		backend.cancelCurrentConnection = nil
		backend.broadcastStatusUpdate()
	}
	backend.broadcastStatusUpdate()

	tx := backend.broker.Sub(brokerTopicTx)

	go func() {
		defer backend.cancelCurrentConnection()
		reader.ReadFromSerial(ctx, backend.log, port, tx, onReceive)
	}()

	return nil
}

func (backend *DeviceBackend) connectToFirstIfNotConnected() {
	if backend.cancelCurrentConnection != nil {
		// already connected, nothing to do
		return
	}

	devices := backend.enumerator.ListMatchingDevices()

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

// Check if client has requested manual-connect via a Sec-WebSocket-Protocol
func wantsManualConnect(req *http.Request) bool {
	for _, protocol := range gorilla.Subprotocols(req) {
		if protocol == "manual-connect" {
			return true
		}
	}
	return false
}

func (backend *DeviceBackend) RegisterSubscriber(req *http.Request) {
	backend.subscriberCount++

	// If a client has specified manual-connect in WebSocket sub-protocols,
	// we disable auto-connect globally. Last-client-wins, meaning that
	// if another client connects later without `manual-connect`, then
	// auto-connect will be re-enabled.
	if wantsManualConnect(req) {
		backend.disableAutoConnect()
	} else {
		// backwards compatible setup: auto-connect by default
		backend.connectToFirstIfNotConnected()
		backend.enableAutoConnect()
	}
}

// Deregister subscribers and disconnect when none left
func (backend *DeviceBackend) DeregisterSubscriber(req *http.Request) {
	backend.subscriberCount--

	if backend.subscriberCount == 0 {
		backend.disableAutoConnect()
		backend.Disconnect()
	}
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
	devices := backend.enumerator.ListMatchingDevices()
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
	matching := backend.enumerator.ListMatchingDevices()
	devices := make(chan websocket.DeviceInfo)

	go func(usbDevices []websocket.UsbDeviceInfo) {
		for _, usbDevice := range usbDevices {
			// Terminate if we have been cancelled
			if ctx.Err() != nil {
				break
			}

			usbDevice := usbDevice // copy to ref-by-value, yay golang
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
