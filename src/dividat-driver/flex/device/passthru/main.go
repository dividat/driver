// Passthru device for sending chunked serial bytes "as is"
// Useful for:
// - recording raw serial data from a device
// - replaying recoreded raw serial data
//
// Currently used in unit tests.
package passthru

import (
	"bufio"
	"context"
	"io"

	"github.com/sirupsen/logrus"
	"go.bug.st/serial"
)

// Serial communication
type PassthruHandler struct{}

func (PassthruHandler) Run(ctx context.Context, logger *logrus.Entry, port serial.Port, tx chan interface{}, onReceive func([]byte)) {
	logger.Info("PassthruHandler started")
	readerCtx := context.WithoutCancel(ctx)

	// Channel to receive ack that reader is done
	readerDoneChan := make(chan struct{})

	// Start the initial reader goroutine
	go readFromPort(readerCtx, logger, port, onReceive, readerDoneChan)

	// Forward WebSocket commands to device
	for {
		select {
		case <-ctx.Done():
			return

		case <-readerDoneChan:
			return

		case i := <-tx:
			data, _ := i.([]byte)
			_, err := port.Write(data)
			if err != nil {
				logger.WithField("error", err).Info("Failed to write command to serial port.")
				return
			}
			logger.WithField("bytes", data).Debug("Wrote binary command to serial out: " + string(data))
		}
	}
}

// Infinite loop for requesting and reading serial data.
// Stops (returns) upon any error or ctx cancel.
func readFromPort(
	ctx context.Context,
	logger *logrus.Entry,
	port serial.Port,
	onReceive func([]byte),
	doneChan chan<- struct{},
) {
	defer func() {
		// Signal that the reader has completed
		close(doneChan)
	}()

	// max estimated frame size in bytes:
	// 10 (header) + 24*24*4 (samples) + 20 (misc extras) = 2380 bytes, so ~2 kilobytes
	reader := bufio.NewReaderSize(port, 2048)

	var message []byte = make([]byte, 2048)
	// Start signal acquisition
	for {
		// Terminate if we were cancelled
		if ctx.Err() != nil {
			logger.Debug("Stopping reader: context cancelled")
			return
		}

		readBytes, err := io.ReadAtLeast(reader, message, 1)
		if err != nil {
			logger.WithField("err", err).Error("Error reading from serial port")
			return
		}
		onReceive(message[:readBytes])
	}
}
