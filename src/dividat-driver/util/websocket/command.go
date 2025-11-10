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

// A broadcast is a Message that it sent to all connected clients
type Broadcast struct {
	message Message
}

func (broadcast *Broadcast) MarshalJSON() ([]byte, error) {
	temp := struct {
		Type    string  `json:"type"`
		Message Message `json:"message"`
	}{}
	temp.Type = "broadcast"
	temp.Message = broadcast.message

	return json.Marshal(&temp)
}

// Driver Message sent to Play in response to a Command (hence, to a single client)
type Message struct {
	*Status
	Discovered            *DeviceInfo
	FirmwareUpdateMessage *FirmwareUpdateMessage
}

type DeviceInfo struct {
	TcpDeviceInfo *zeroconf.ServiceEntry `json:"tcpDevice"`
	UsbDeviceInfo *UsbDeviceInfo         `json:"usbDevice"`
}

type UsbDeviceInfo struct {
	Path string `json:"path"`

	IdVendor  uint16 `json:"idVendor"`
	IdProduct uint16 `json:"idProduct"`
	BcdDevice uint16 `json:"bcdDevice"`

	SerialNumber string `json:"serialNumber"`
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
}

// Status is a message containing status information
type Status struct {
	// ip for Senso, /dev/* path for Flex
	Address *string
	// optional, currently only used in Flex
	DeviceInfo *DeviceInfo
}

type FirmwareUpdateMessage struct {
	FirmwareUpdateProgress *string
	FirmwareUpdateSuccess  *string
	FirmwareUpdateFailure  *string
}

// MarshalJSON ipmlements JSON encoder for messages
func (message *Message) MarshalJSON() ([]byte, error) {
	if message.Status != nil {
		status := struct {
			Type       string      `json:"type"`
			Address    *string     `json:"address"`
			DeviceInfo *DeviceInfo `json:"deviceInfo"`
		}{
			Type:       "Status",
			Address:    message.Status.Address,
			DeviceInfo: message.Status.DeviceInfo,
		}
		return json.Marshal(&status)

	} else if message.Discovered != nil {
		serviceEntry := message.Discovered.TcpDeviceInfo
		msg := struct {
			Type string `json:"type"`
			// Senso only
			ServiceEntry *zeroconf.ServiceEntry `json:"service"`
			IP           []net.IP               `json:"ip"`
			// Flex only
			UsbDevice *UsbDeviceInfo `json:"usbDevice"`
		}{
			Type:         "Discovered",
			ServiceEntry: serviceEntry,
			UsbDevice:    message.Discovered.UsbDeviceInfo,
		}
		if serviceEntry != nil {
			msg.IP = append(serviceEntry.AddrIPv4, serviceEntry.AddrIPv6...)
		}
		return json.Marshal(&msg)

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
