// common parts to senso.websocket and flex.websocket
package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/cskr/pubsub"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/dividat/driver/src/dividat-driver/service"
)

type SendMsg struct {
	Progress func(string)
	Failure  func(string)
	Success  func(string)
}

type DeviceBackend interface {
	// TODO: will not work for Flex
	Address() *string
	Discover(duration int, ctx context.Context, log *logrus.Entry) chan service.Service
	Connect(address string)
	Disconnect()
	RegisterSubscriber()
	DeregisterSubscriber()
	ProcessFirmwareUpdateRequest(command UpdateFirmware, send SendMsg)
	IsUpdatingFirmware() bool
}

type Handle struct {
	Broker   *pubsub.PubSub
	BrokerRx string
	BrokerTx string

	Log *logrus.Entry

	DeviceBackend DeviceBackend
}

func (handle *Handle) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// Set up logger
	var log = handle.Log.WithFields(logrus.Fields{
		"clientAddress": r.RemoteAddr,
		"userAgent":     r.UserAgent(),
	})

	// Update to WebSocket
	conn, err := webSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.WithError(err).Error("Could not upgrade connection to WebSocket.")
		http.Error(w, "WebSocket upgrade error", http.StatusBadRequest)
		return
	}

	log.Info("WebSocket connection opened")

	// Create a mutex for writing to WebSocket (connection supports only one concurrent reader and one concurrent writer (https://godoc.org/github.com/gorilla/websocket#hdr-Concurrency))
	writeMutex := sync.Mutex{}

	// Create a context for this WebSocket connection
	ctx, cancel := context.WithCancel(context.Background())

	// Send binary data up the WebSocket
	sendBinary := func(data []byte) error {
		writeMutex.Lock()
		conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
		err := conn.WriteMessage(websocket.BinaryMessage, data)
		writeMutex.Unlock()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.WithError(err).Error("WebSocket error")
			}
			return err
		}
		return nil
	}

	// send messgae up the WebSocket
	sendMessage := func(message Message) error {
		writeMutex.Lock()
		conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
		err := conn.WriteJSON(&message)
		writeMutex.Unlock()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.WithError(err).Error("WebSocket error")
			}
			return err
		}
		return nil
	}

	// Create channels with data received from device
	rx := handle.Broker.Sub(handle.BrokerRx)

	// TODO: remove once Flex handles commands
	handle.DeviceBackend.RegisterSubscriber()

	// send data from Control and Data channel
	go rx_data_loop(ctx, rx, sendBinary)

	// Helper function to close the connection
	close := func() {
		// Unsubscribe from broker
		handle.Broker.Unsub(rx)

		// TODO: remove once Flex handles commands
		handle.DeviceBackend.DeregisterSubscriber()

		// Cancel the context
		cancel()

		// Close websocket connection
		conn.Close()

		log.Info("Websocket connection closed")
	}

	// Main loop for the WebSocket connection
	go func() {
		defer close()
		for {

			messageType, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.WithError(err).Error("WebSocket error")
				}
				return
			}

			if messageType == websocket.BinaryMessage {

				if handle.DeviceBackend.IsUpdatingFirmware() {
					handle.Log.Debug("Ignoring device command during firmware update.")
					continue
				}

				handle.Broker.TryPub(msg, handle.BrokerTx)

			} else if messageType == websocket.TextMessage {

				var command Command
				decodeErr := json.Unmarshal(msg, &command)
				if decodeErr != nil {
					log.WithField("rawCommand", msg).WithError(decodeErr).Warning("Can not decode command.")
					continue
				}
				log.WithField("command", prettyPrintCommand(command)).Debug("Received command.")

				if handle.DeviceBackend.IsUpdatingFirmware() && (command.GetStatus == nil || command.Discover == nil) {
					log.WithField("command", prettyPrintCommand(command)).Debug("Ignoring command during firmware update.")
					continue
				}

				err := handle.dispatchCommand(ctx, log, command, sendMessage)
				if err != nil {
					return
				}
			}

		}
	}()

}

// HELPERS

// dispatchCommand handles incomming commands and sends responses back up the WebSocket
func (handle *Handle) dispatchCommand(ctx context.Context, log *logrus.Entry, command Command, sendMessage func(Message) error) error {

	if command.GetStatus != nil {
		// TODO: think about a general Status interface
		var message Message

		message.Status = &Status{Address: handle.DeviceBackend.Address()}

		err := sendMessage(message)

		if err != nil {
			return err
		}

	} else if command.Connect != nil {
		handle.DeviceBackend.Connect(command.Connect.Address)
		return nil

	} else if command.Disconnect != nil {
		handle.DeviceBackend.Disconnect()
		return nil

	} else if command.Discover != nil {
		entries := handle.DeviceBackend.Discover(command.Discover.Duration, ctx, log)

		// TODO: the async interface makes little sense for Flex
		go func(entries chan service.Service) {
			for entry := range entries {
				log.WithField("service", entry).Debug("Discovered service.")

				var message Message
				message.Discovered = &entry.ServiceEntry

				err := sendMessage(message)
				if err != nil {
					return
				}

			}
			log.Debug("Discovery finished.")
			// TODO: client needs to know it's finished too!
		}(entries)

		return nil

	} else if command.UpdateFirmware != nil {
		go handle.DeviceBackend.ProcessFirmwareUpdateRequest(*command.UpdateFirmware, SendMsg{
			Progress: func(msg string) {
				sendMessage(firmwareUpdateProgress(msg))
			},
			Failure: func(msg string) {
				sendMessage(firmwareUpdateFailure(msg))
			},
			Success: func(msg string) {
				sendMessage(firmwareUpdateSuccess(msg))
			},
		})
	}
	return nil
}

func firmwareUpdateSuccess(msg string) Message {
	return firmwareUpdateMessage(FirmwareUpdateMessage{FirmwareUpdateSuccess: &msg})
}

func firmwareUpdateFailure(msg string) Message {
	return firmwareUpdateMessage(FirmwareUpdateMessage{FirmwareUpdateFailure: &msg})
}

func firmwareUpdateProgress(msg string) Message {
	return firmwareUpdateMessage(FirmwareUpdateMessage{FirmwareUpdateProgress: &msg})
}

func firmwareUpdateMessage(msg FirmwareUpdateMessage) Message {
	return Message{FirmwareUpdateMessage: &msg}
}

// rx_data_loop reads data from device and forwards it up the WebSocket
func rx_data_loop(ctx context.Context, rx chan interface{}, send func([]byte) error) {
	var err error
	for {
		select {
		case <-ctx.Done():
			return

		case i := <-rx:
			data, ok := i.([]byte)
			if ok {
				err = send(data)
			}
		}

		if err != nil {
			return
		}
	}
}

// Helper to upgrade http to WebSocket
var webSocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Check is performed by top-level HTTP middleware, and not repeated here.
		return true
	},
}
