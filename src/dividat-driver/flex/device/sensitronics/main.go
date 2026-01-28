package sensitronics

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
	"go.bug.st/serial"
)

const (
	HEADER_START_MARKER = 0xFF
	HEADER_SIZE         = 4
)

type SensitronicsHandler struct{}

func (SensitronicsHandler) Run(ctx context.Context, logger *logrus.Entry, port serial.Port, tx chan interface{}, onReceive func([]byte)) {
	readerCtx := context.WithoutCancel(ctx)

	port.ResetInputBuffer() // flush any unread data buffered by the OS

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

func readMessage(reader *bufio.Reader) ([]byte, error) {
	// Read start marker
	marker, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	if marker != HEADER_START_MARKER {
		return nil, fmt.Errorf("expected header start marker 0x%02X, got 0x%02X", HEADER_START_MARKER, marker)
	}

	// Read message type
	messageType, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	// Read message length
	var lengthBytes [2]byte
	if _, err := io.ReadFull(reader, lengthBytes[:]); err != nil {
		return nil, err
	}
	bodyLength := int(binary.LittleEndian.Uint16(lengthBytes[:]))

	// Allocate full message buffer and reassemble header
	message := make([]byte, HEADER_SIZE+bodyLength)
	message[0] = marker
	message[1] = messageType
	message[2] = lengthBytes[0]
	message[3] = lengthBytes[1]

	// Read body directly into message buffer
	if _, err := io.ReadFull(reader, message[HEADER_SIZE:]); err != nil {
		return nil, err
	}

	return message, nil
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

	reader := bufio.NewReader(port)

	START_MEASUREMENT_CMD := []byte{'S', '\n'}
	_, err := port.Write(START_MEASUREMENT_CMD)
	if err != nil {
		logger.WithField("error", err).Info("Failed to write start message to serial port.")
		return
	}

	// Start signal acquisition
	for {
		// Terminate if we were cancelled
		if ctx.Err() != nil {
			logger.Debug("Stopping reader: context cancelled")
			return
		}

		message, err := readMessage(reader)
		if err != nil {
			logger.WithField("err", err).Error("Error reading from serial port")
			return
		}

		onReceive(message)
	}
}
