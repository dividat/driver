package senso

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/dividat/driver/src/dividat-driver/firmware"
	"github.com/dividat/driver/src/dividat-driver/protocol"
	"github.com/dividat/driver/src/dividat-driver/websocket"
)

// Disconnect from current connection
func (backend *DeviceBackend) ProcessFirmwareUpdateRequest(command protocol.UpdateFirmware, send websocket.SendMsg) {
	backend.log.Info("Processing firmware update request.")
	backend.firmwareUpdate.SetUpdating(true)

	if backend.cancelCurrentConnection != nil {
		send.Progress("Disconnecting from the Senso")
		backend.cancelCurrentConnection()
	}

	image, err := decodeImage(command.Image)
	if err != nil {
		msg := fmt.Sprintf("Error decoding base64 string: %v", err)
		send.Failure(msg)
		backend.log.Error(msg)
		return
	}

	err = firmware.UpdateBySerial(context.Background(), command.SerialNumber, image, send.Progress)
	if err != nil {
		failureMsg := fmt.Sprintf("Failed to update firmware: %v", err)
		send.Failure(failureMsg)
		backend.log.Error(failureMsg)
	} else {
		send.Success("Firmware successfully transmitted")
	}
	backend.firmwareUpdate.SetUpdating(false)
}

func decodeImage(base64Str string) (io.Reader, error) {
	data, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
