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

// Serial communication

const (
	HEADER_START_MARKER    = 0xFF
	HEADER_TYPE_TERMINATOR = 0xA // newline
)

type SensitronicsReader struct{}

func (SensitronicsReader) ReadFromSerial(ctx context.Context, logger *logrus.Entry, port serial.Port, tx chan interface{}, onReceive func([]byte)) {
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
	// read header start marker
	marker, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	if marker != HEADER_START_MARKER {
		return nil, fmt.Errorf("Expected header start marker, got %u", marker)
	}

	// read type from header, return will include the HEADER_TYPE_TERMINATOR
	messageType, err := reader.ReadBytes(HEADER_TYPE_TERMINATOR)
	if err != nil {
		return nil, err
	}

	messageLengthBytesLE := make([]byte, 2, 2)
	_, err = io.ReadFull(reader, messageLengthBytesLE)
	if err != nil {
		return nil, err
	}

	messageLength := uint(binary.LittleEndian.Uint16(messageLengthBytesLE))

	messageBody := make([]byte, messageLength, messageLength)
	_, err = io.ReadFull(reader, messageBody)
	if err != nil {
		return nil, err
	}

	// TODO: bump go to 1.22 and replace with slices.Concat or otherwise
	totalLength := 1 + len(messageType) + len(messageLengthBytesLE) + len(messageBody)
	fullMessage := make([]byte, 0, totalLength)
	fullMessage = append(fullMessage, marker)
	fullMessage = append(fullMessage, messageType...)
	fullMessage = append(fullMessage, messageLengthBytesLE...)
	fullMessage = append(fullMessage, messageBody...)

	return fullMessage, nil
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
