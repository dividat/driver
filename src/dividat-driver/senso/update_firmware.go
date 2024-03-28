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

func (handle *Handle) isUpdatingFirmware() bool {
	handle.firmwareUpdateMutex.Lock()
	state := handle.firmwareUpdateInProgress
	handle.firmwareUpdateMutex.Unlock()
	return state
}

func (handle *Handle) setUpdatingFirmware(state bool) {
	handle.firmwareUpdateMutex.Lock()
	handle.firmwareUpdateInProgress = state
	handle.firmwareUpdateMutex.Unlock()
}

// Disconnect from current connection
func (handle *Handle) ProcessFirmwareUpdateRequest(command UpdateFirmware, onUpdate OnUpdate) {
	fmt.Println("Processing firmware update request.")
	handle.log.Info("Processing firmware update request.")
	handle.setUpdatingFirmware(true)
	if handle.cancelCurrentConnection != nil {
		handle.cancelCurrentConnection()
	}
	image, err := decodeImage(command.Image)
	if err != nil {
		msg := fmt.Sprintf("Error decoding base64 string: %v", err)
		onUpdate(FirmwareUpdateMessage{FirmwareUpdateFailure: &msg})
		handle.log.Error(msg)
	}

	onProgress := func(progressMsg string) {
		onUpdate(FirmwareUpdateMessage{FirmwareUpdateProgress: &progressMsg})
	}

	err = firmware.Update(context.Background(), image, nil, &command.Address, onProgress)
	if err != nil {
		failureMsg := fmt.Sprintf("Failed to update firmware: %v", err)
		onUpdate(FirmwareUpdateMessage{FirmwareUpdateFailure: &failureMsg})
		handle.log.Error(failureMsg)
		return
	}
	successMsg := "Firmware successfully transmitted."
	onUpdate(FirmwareUpdateMessage{FirmwareUpdateSuccess: &successMsg})
	handle.setUpdatingFirmware(false)
}

func decodeImage(base64Str string) (io.Reader, error) {
	data, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
