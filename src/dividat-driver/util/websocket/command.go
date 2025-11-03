package websocket

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/libp2p/zeroconf/v2"
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
	} else if command.UpdateFirmware != nil {
		return "UpdateFirmware"
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
	SerialNumber string `json:"serialNumber"`
	Image        string `json:"image"`
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
