package sensitronics

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"slices"

	"github.com/sirupsen/logrus"
	"go.bug.st/serial"
)

// Serial communication

const (
	SENSITRONICS_DRIVER_HEADER = "V2" // TODO define something more appropriate
	HEADER_START_MARKER        = 0xFF
	HEADER_TYPE_TERMINATOR     = 0xA // newline
	HEADER_TYPE_FRAME          = "FS"
	HEADER_TYPE_COMMAND_REPLY  = "CR"
	COMMAND_REPLY_TERMINATOR   = 0xA // newline
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

func readFrame(reader *bufio.Reader) ([]byte, error) {
	length_msb, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	length_lsb, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	samplesInSet := int(binary.BigEndian.Uint16([]byte{length_msb, length_lsb}))
	bufSize := samplesInSet * (1 + 1 + 2)
	frameBuf := make([]byte, bufSize, bufSize)
	_, err = io.ReadFull(reader, frameBuf)
	if err != nil {
		return nil, err
	} else {
		length := []byte{length_msb, length_lsb}
		return append(length, frameBuf...), nil
	}
}

func readCommandReply(reader *bufio.Reader) ([]byte, error) {
	command, err := reader.ReadBytes(COMMAND_REPLY_TERMINATOR)
	if err != nil {
		return nil, err
	} else {
		return append(command, COMMAND_REPLY_TERMINATOR), nil
	}
}

func readUnknownMessage(reader *bufio.Reader) ([]byte, error) {
	unknown, err := reader.ReadBytes(0xA)
	if err != nil {
		return nil, err
	} else {
		return append(unknown, 0xA), nil
	}
}

type MessageType string

func readHeader(reader *bufio.Reader) (MessageType, error) {
	// read header start marker
	marker, err := reader.ReadByte()

	if err != nil {
		return "", err
	}

	if marker != HEADER_START_MARKER {
		return "", errors.New("Expected header start marker")
	}

	// read type from header
	messageType, err := reader.ReadBytes(HEADER_TYPE_TERMINATOR)

	if err != nil {
		return "", err
	}
	return MessageType(string(messageType)), nil

}

func serializeHeader(messageType MessageType) []byte {
	return slices.Concat([]byte{HEADER_START_MARKER}, []byte(messageType), []byte{HEADER_TYPE_TERMINATOR})
}

func serializeMessage(messageType MessageType, message []byte) []byte {
	driverHeader := []byte(SENSITRONICS_DRIVER_HEADER)
	firmwareHeader := serializeHeader(messageType)
	return slices.Concat(driverHeader, firmwareHeader, message)
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

		messageType, err := readHeader(reader)
		if err != nil {
			logger.WithField("err", err).Error("Error reading from serial port")
			return
		}

		switch messageType {
		case HEADER_TYPE_FRAME:
			frame, err := readFrame(reader)
			if err != nil {
				logger.WithField("err", err).Error("Error reading from serial port")
				return
			}
			msg := serializeMessage(messageType, frame)
			onReceive(msg)
		case HEADER_TYPE_COMMAND_REPLY:
			commandReply, err := readCommandReply(reader)
			if err != nil {
				logger.WithField("err", err).Error("Error reading from serial port")
				return
			}
			msg := serializeMessage(messageType, commandReply)
			onReceive(msg)
		default:
			unknown, err := readUnknownMessage(reader)
			if err != nil {
				logger.WithField("err", err).Error("error reading from serial port")
				return
			}
			logger.WithField("msgType", messageType).WithField("msgBody", unknown).Warn("Unknown message type")
		}
	}
}
