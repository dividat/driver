package sensitronics

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"slices"

	"github.com/sirupsen/logrus"
	"go.bug.st/serial"
)

// Serial communication

const (
	DRIVER_PROTOCOL_VERSION = 0x02
	HEADER_START_MARKER     = 0xFF
	HEADER_TYPE_TERMINATOR  = 0xA // newline
)

// Actually attempt to connect to an individual serial port and pipe its signal into the callback, summarizing
// package units into a buffer.
func ConnectSerial(ctx context.Context, logger *logrus.Entry, serialName string, tx chan interface{}, onReceive func([]byte)) {
	mode := &serial.Mode{
		BaudRate: 115200,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	logger.WithField("name", serialName).Info("Attempting to connect with serial port.")
	port, err := serial.Open(serialName, mode)
	if err != nil {
		logger.WithField("config", mode).WithField("error", err).Info("Failed to open connection to serial port.")
		return
	}
	port.ResetInputBuffer() // flush any unread data buffered by the OS

	readerCtx, readerCtxCancel := context.WithCancel(ctx)

	defer func() {
		logger.WithField("name", serialName).Info("Disconnecting from serial port.")
		port.Close()
		readerCtxCancel()
	}()

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
			_, err = port.Write(data)
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

	return slices.Concat([]byte{marker}, messageType, messageLengthBytesLE, messageBody), nil
}

func addDriverHeader(message []byte) []byte {
	driverHeader := []byte{DRIVER_PROTOCOL_VERSION}
	return append(driverHeader, message...)
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

		messageWithDriverHeader := addDriverHeader(message)
		onReceive(messageWithDriverHeader)
	}
}
