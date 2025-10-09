package flex

/* Connects to Senso Flex devices through a serial connection and chunks serial
data into self-contained messages to be delivered over a WebSocket.

The functionality of this module is as follows:

- While connected, scan for serial devices that look like a potential Flex device
- Connect to a suitable serial device and start reading serial data
- Minimally parse incoming data to determine start and end of a message
- Tag the message with a DRIVER_PROTOCOL_VERSION
- Send each complete message set to client as a binary package

The module also forwards any incomding serial commands from the WebSocket to the
serial device "as is".

For Sensing Tex controllers, the serial data is parsed and only the frame samples are send over the WebSocket. DRIVER_PROTOCOL_VERSION = 0x01

For Sensitronics controllers, the messages are chunked in an opaque way and all valid messages get sent over the WebSocket. DRIVER_PROTOCOL_VERSION = 0x02

*/

import (
	"context"
	"strings"
	"time"

	"github.com/cskr/pubsub"
	"github.com/sirupsen/logrus"
	"go.bug.st/serial/enumerator"

	"github.com/dividat/driver/src/dividat-driver/flex/sensing_tex"
	"github.com/dividat/driver/src/dividat-driver/flex/sensitronics"
)

// Handle for managing SensingTex connection
type Handle struct {
	broker *pubsub.PubSub

	ctx context.Context

	cancelCurrentConnection context.CancelFunc
	subscriberCount         int

	log *logrus.Entry
}

// New returns an initialized handler
func New(ctx context.Context, log *logrus.Entry) *Handle {
	handle := Handle{
		broker: pubsub.New(32),
		ctx:    ctx,
		log:    log,
	}

	// Clean up
	go func() {
		<-ctx.Done()
		handle.broker.Shutdown()
	}()

	return &handle
}

// Connect to device
func (handle *Handle) Connect() {
	handle.subscriberCount++

	// If there is no existing connection, create it
	if handle.cancelCurrentConnection == nil {
		ctx, cancel := context.WithCancel(handle.ctx)

		onReceive := func(data []byte) {
			handle.broker.TryPub(data, "flex-rx")
		}

		go listeningLoop(ctx, handle.log, handle.broker.Sub("flex-tx"), onReceive)

		handle.cancelCurrentConnection = cancel
	}
}

// Deregister subscribers and disconnect when none left
func (handle *Handle) DeregisterSubscriber() {
	handle.subscriberCount--

	if handle.subscriberCount == 0 && handle.cancelCurrentConnection != nil {
		handle.cancelCurrentConnection()
		handle.cancelCurrentConnection = nil
	}
}

// Keep looking for serial devices and connect to them when found, sending signals into the
// callback.
func listeningLoop(ctx context.Context, logger *logrus.Entry, tx chan interface{}, onReceive func([]byte)) {
	for {
		scanAndConnectSerial(ctx, logger, tx, onReceive)

		// Terminate if we were cancelled
		if ctx.Err() != nil {
			return
		}

		time.Sleep(2 * time.Second)
	}
}

// One pass of browsing for serial devices and trying to connect to them turn by turn, first
// successful connection wins.
func scanAndConnectSerial(ctx context.Context, logger *logrus.Entry, tx chan interface{}, onReceive func([]byte)) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		logger.WithField("error", err).Info("Could not list serial devices.")
		return
	}

	for _, port := range ports {
		// Terminate if we have been cancelled
		if ctx.Err() != nil {
			return
		}

		logger.WithField("name", port.Name).WithField("vendor", port.VID).WithField("product", port.Product).Debug("Considering serial port.")

		if isSensitronicsLike(port) {
			sensitronics.ConnectSerial(ctx, logger, port.Name, tx, onReceive)
		} else if isFlexLike(port) {
			sensing_tex.ConnectSerial(ctx, logger, port.Name, tx, onReceive)
		}
	}
}

// Check whether a port looks like a potential Flex device.
//
// Vendor IDs:
//
//	16C0 - Van Ooijen Technische Informatica (Teensy)
func isFlexLike(port *enumerator.PortDetails) bool {
	vendorId := strings.ToUpper(port.VID)

	return vendorId == "16C0"
}

// Check whether a port looks like a potential Sensitronics device.
//
// Vendor IDs:
//
//	16C0 - Van Ooijen Technische Informatica (Teensy)
//
// Product name: Dividat16x16
// Manufacturer name: Sensitronics (not used in the check)
func isSensitronicsLike(port *enumerator.PortDetails) bool {
	vendorId := strings.ToUpper(port.VID)
	product := strings.ToUpper(port.Product)

	return vendorId == "16C0" && product == "DIVIDAT16X16"
}
