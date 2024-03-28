package senso

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/grandcat/zeroconf"
	"github.com/sirupsen/logrus"
)

// WEBSOCKET PROTOCOL

// Command sent by Play
type Command struct {
	*GetStatus

	*Connect
	*Disconnect

	*Discover
	*UpdateFirmware
}

func prettyPrintCommand(command Command) string {
	if command.GetStatus != nil {
		return "GetStatus"
	} else if command.Connect != nil {
		return "Connect"
	} else if command.Disconnect != nil {
		return "Disconnect"
	} else if command.Discover != nil {
		return "Discover"
	}
	return "Unknown"
}

// GetStatus command
type GetStatus struct{}

// Connect command
type Connect struct {
	Address string `json:"address"`
}

// Disconnect command
type Disconnect struct{}

// Discover command
type Discover struct {
	Duration int `json:"duration"`
}

type UpdateFirmware struct {
	Address string `json:"address"`
	Image   string `json:"image"`
}

// UnmarshalJSON implements encoding/json Unmarshaler interface
func (command *Command) UnmarshalJSON(data []byte) error {

	// Helper struct to get type
	temp := struct {
		Type string `json:"type"`
	}{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if temp.Type == "GetStatus" {
		command.GetStatus = &GetStatus{}

	} else if temp.Type == "Connect" {
		err := json.Unmarshal(data, &command.Connect)
		if err != nil {
			return err
		}

	} else if temp.Type == "Disconnect" {
		command.Disconnect = &Disconnect{}

	} else if temp.Type == "Discover" {

		err := json.Unmarshal(data, &command.Discover)
		if err != nil {
			return err
		}

	} else if temp.Type == "UpdateFirmware" {
		err := json.Unmarshal(data, &command.UpdateFirmware)
		if err != nil {
			return err
		}
	} else {
		return errors.New("can not decode unknown command")
	}

	return nil
}

// Message that can be sent to Play
type Message struct {
	*Status
	Discovered            *zeroconf.ServiceEntry
	FirmwareUpdateMessage *FirmwareUpdateMessage
}

// Status is a message containing status information
type Status struct {
	Address *string
}

type FirmwareUpdateMessage struct {
	FirmwareUpdateProgress *string
	FirmwareUpdateSuccess  *string
	FirmwareUpdateFailure  *string
}

// MarshalJSON ipmlements JSON encoder for messages
func (message *Message) MarshalJSON() ([]byte, error) {
	if message.Status != nil {
		return json.Marshal(&struct {
			Type    string  `json:"type"`
			Address *string `json:"address"`
		}{
			Type:    "Status",
			Address: message.Status.Address,
		})

	} else if message.Discovered != nil {
		return json.Marshal(&struct {
			Type         string                 `json:"type"`
			ServiceEntry *zeroconf.ServiceEntry `json:"service"`
			IP           []net.IP               `json:"ip"`
		}{
			Type:         "Discovered",
			ServiceEntry: message.Discovered,
			IP:           append(message.Discovered.AddrIPv4, message.Discovered.AddrIPv6...),
		})

	} else if message.FirmwareUpdateMessage != nil {
		fwUpdate := struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}{}

		firmwareUpdateMessage := *message.FirmwareUpdateMessage

		if firmwareUpdateMessage.FirmwareUpdateProgress != nil {

			fwUpdate.Type = "FirmwareUpdateProgress"
			fwUpdate.Message = *firmwareUpdateMessage.FirmwareUpdateProgress

		} else if firmwareUpdateMessage.FirmwareUpdateFailure != nil {

			fwUpdate.Type = "FirmwareUpdateFailure"
			fwUpdate.Message = *firmwareUpdateMessage.FirmwareUpdateFailure

		} else if firmwareUpdateMessage.FirmwareUpdateSuccess != nil {

			fwUpdate.Type = "FirmwareUpdateSuccess"
			fwUpdate.Message = *firmwareUpdateMessage.FirmwareUpdateSuccess

		} else {
			return nil, errors.New("could not marshal firmware update message")
		}

		return json.Marshal(fwUpdate)
	}

	return nil, errors.New("could not marshal message")

}

// Implement net/http Handler interface
func (handle *Handle) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	// Set up logger
	var log = handle.log.WithFields(logrus.Fields{
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

	// Create channels with data received from Senso
	rx := handle.broker.Sub("rx")

	// send data from Control and Data channel
	go rx_data_loop(ctx, rx, sendBinary)

	// Helper function to close the connection
	close := func() {
		// Unsubscribe from broker
		handle.broker.Unsub(rx)

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

			if handle.isUpdatingFirmware() {
				handle.log.Info("Firmware update in progress, ignoring websocket message.")
				continue
			}

			if messageType == websocket.BinaryMessage {
				handle.broker.TryPub(msg, "tx")

			} else if messageType == websocket.TextMessage {

				var command Command
				decodeErr := json.Unmarshal(msg, &command)
				if decodeErr != nil {
					log.WithField("rawCommand", msg).WithError(decodeErr).Warning("Can not decode command.")
					continue
				}
				log.WithField("command", prettyPrintCommand(command)).Debug("Received command.")

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

		var message Message

		message.Status = &Status{Address: handle.Address}

		err := sendMessage(message)

		if err != nil {
			return err
		}

	} else if command.Connect != nil {
		handle.Connect(command.Connect.Address)
		return nil

	} else if command.Disconnect != nil {
		handle.Disconnect()
		return nil

	} else if command.Discover != nil {

		discoveryCtx, _ := context.WithTimeout(ctx, time.Duration(command.Discover.Duration)*time.Second)

		entries := handle.Discover(discoveryCtx)

		go func(entries chan *zeroconf.ServiceEntry) {
			for entry := range entries {
				log.WithField("service", entry).Debug("Discovered service.")

				var message Message
				message.Discovered = entry

				err := sendMessage(message)
				if err != nil {
					return
				}

			}
			log.Debug("Discovery finished.")
		}(entries)

		return nil

	} else if command.UpdateFirmware != nil {
		go func(){
			handle.ProcessFirmwareUpdateRequest(*command.UpdateFirmware, func(msg FirmwareUpdateMessage) {
				sendMessage(Message{
					FirmwareUpdateMessage: &msg,
				})
			})
		}()
	}
	return nil
}

// rx_data_loop reads data from Senso and forwards it up the WebSocket
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
		return true
	},
}
