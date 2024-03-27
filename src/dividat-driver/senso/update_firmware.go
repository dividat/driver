package senso

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/dividat/driver/src/dividat-driver/firmware"
)

type OnUpdate func(msg FirmwareUpdateMessage)

// Disconnect from current connection
func (handle *Handle) ProcessFirmwareUpdateRequest(command UpdateFirmware, onUpdate OnUpdate) {
	handle.log.Info("Processing firmware update request.")
	if handle.cancelCurrentConnection != nil {
		handle.cancelCurrentConnection()
	}
	image, err := decodeImage(command.Image)
	if err != nil {
		msg := fmt.Sprintf("Error decoding base64 string: %v", err)
		onUpdate(FirmwareUpdateMessage{FirmwareUpdateFailure: &msg})
		handle.log.Error(msg)
	}

	onProgress := func(progressMsg string){
		onUpdate(FirmwareUpdateMessage{FirmwareUpdateProgress: &progressMsg})
	}

	err = firmware.Update(context.Background(), image, nil, &command.Address, onProgress)
	if err != nil {
		failureMsg := fmt.Sprintf("Failed to update firmware: %v", err)
		onUpdate(FirmwareUpdateMessage{FirmwareUpdateFailure: &failureMsg})
		handle.log.Error(failureMsg)
	}
	successMsg := "Firmware successfully transmitted."
	onUpdate(FirmwareUpdateMessage{FirmwareUpdateSuccess: &successMsg})
}

func decodeImage(base64Str string) (io.Reader, error) {
	data, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
