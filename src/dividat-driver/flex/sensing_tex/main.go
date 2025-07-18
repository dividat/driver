package sensing_tex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"

	"github.com/sirupsen/logrus"
	"go.bug.st/serial"
)

// Serial communication

type ReaderState int

const (
	WAITING_FOR_HEADER ReaderState = iota
	HEADER_START
	HEADER_READ_LENGTH_MSB
	HEADER_READ_LENGTH_LSB
	WAITING_FOR_BODY
	BODY_START
	BODY_READ_SAMPLE
	UNEXPECTED_BYTE
)

const (
	DRIVER_PROTOCOL_VERSION = 0x01
)

const (
	HEADER_START_MARKER = 'N'
	BODY_START_MARKER   = 'P'
)

const (
	// row, column and pressure value, one uint8 each
	BYTES_PER_SAMPLE_8BIT = 3

	// same as above, but pressure value is 2 bytes (uint16), big-endian
	// Note: Sensing Tex docs state value max is 2^12-1 (hence "12bit"),
	// but in practice they seem to send values up to ~9000, so more like
	// "14 bit".
	BYTES_PER_SAMPLE_12BIT = 4
)

var BITDEPTH_8_CMD = []byte{'U', 'L', '\n'}

var BITDEPTH_12_CMD = []byte{'U', 'M', '\n'}

func isBitdepthCommand(cmd []byte) bool {
	return bytes.Equal(cmd, BITDEPTH_8_CMD) || bytes.Equal(cmd, BITDEPTH_12_CMD)
}

func bitdepthCommandToBytesPerSample(cmd []byte) int {
	if bytes.Equal(cmd, BITDEPTH_8_CMD) {
		return BYTES_PER_SAMPLE_8BIT
	} else {
		return BYTES_PER_SAMPLE_12BIT
	}
}

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

	// For backwards compatibility, we set 8bit depth by default
	_, err = port.Write(BITDEPTH_8_CMD)
	if err != nil {
		logger.WithField("error", err).Info("Failed to set bitdepth of 8.")
		return
	}
	configuredBytesPerSample := BYTES_PER_SAMPLE_8BIT

	// Channel to receive ack that reader is done
	readerDoneChan := make(chan struct{})

	// Start the initial reader goroutine
	go readFromPort(readerCtx, logger, port, configuredBytesPerSample, onReceive, readerDoneChan)

	// Forward WebSocket commands to device
	for {
		select {
		case <-ctx.Done():
			return

		case <-readerDoneChan:
			return

		case i := <-tx:
			data, _ := i.([]byte)

			if isBitdepthCommand(data) {
				newBytesPerSample := bitdepthCommandToBytesPerSample(data)

				// If bytes per sample has changed, we need to restart the reader
				if newBytesPerSample != configuredBytesPerSample {
					logger.WithFields(logrus.Fields{
						"old": configuredBytesPerSample,
						"new": newBytesPerSample,
					}).Info("Bytes per sample changed, will restart reader")

					logger.Debug("Sending stop to reader and waiting for ack")
					readerCtxCancel()
					<-readerDoneChan
					logger.Debug("Ack received, reader stopped")

					_, err = port.Write(data)
					if err != nil {
						logger.WithField("error", err).Info("Failed to write new bitdepth.")
						return
					}

					port.ResetInputBuffer() // flush any data that was not yet read

					configuredBytesPerSample = newBytesPerSample

					readerCtx, readerCtxCancel = context.WithCancel(ctx)
					readerDoneChan = make(chan struct{})

					// Start a new reader goroutine with updated bytesPerSample
					go readFromPort(readerCtx, logger, port, configuredBytesPerSample, onReceive, readerDoneChan)
				}
			} else {
				// For non-bitdepth commands, just forward them
				_, err = port.Write(data)
				if err != nil {
					logger.WithField("error", err).Info("Failed to write command to serial port.")
					return
				}
				logger.WithField("bytes", data).Debug("Wrote binary command to serial out: " + string(data))
			}
		}
	}
}

// Infinite loop for requesting and reading serial data.
// Stops (returns) upon any error or ctx cancel.
func readFromPort(
	ctx context.Context,
	logger *logrus.Entry,
	port serial.Port,
	bytesPerSample int,
	onReceive func([]byte),
	doneChan chan<- struct{},
) {
	defer func() {
		// Signal that the reader has completed
		close(doneChan)
	}()

	reader := bufio.NewReader(port)
	state := WAITING_FOR_HEADER
	var samplesLeftInSet int
	var bytesLeftInSample int

	// Note: for Flex v4 this command seems to cause the firmware to push
	// data as fast as we can consume it, whereas for Flex v5 it merely
	// requests a single frame.
	START_MEASUREMENT_CMD := []byte{'S', '\n'}
	_, err := port.Write(START_MEASUREMENT_CMD)
	if err != nil {
		logger.WithField("error", err).Info("Failed to write start message to serial port.")
		return
	}

	// Start signal acquisition
	var buff []byte
	for {
		// Terminate if we were cancelled
		if ctx.Err() != nil {
			logger.Debug("Stopping reader: context cancelled")
			return
		}

		input, err := reader.ReadByte()
		if err != nil {
			logger.WithField("err", err).Error("Error reading from serial port")
			return
		}

		var length_msb byte

		// Finite State Machine for parsing byte stream
		switch {
		case state == WAITING_FOR_HEADER && input == HEADER_START_MARKER:
			state = HEADER_START
		case state == HEADER_START && input == '\n':
			state = HEADER_READ_LENGTH_MSB
		case state == HEADER_READ_LENGTH_MSB:
			// The number of measurements in each set is given as two
			// consecutive bytes (big-endian).
			length_msb = input
			state = HEADER_READ_LENGTH_LSB
		case state == HEADER_READ_LENGTH_LSB:
			length_lsb := input
			samplesLeftInSet = int(binary.BigEndian.Uint16([]byte{length_msb, length_lsb}))
			state = WAITING_FOR_BODY
		case state == WAITING_FOR_BODY && input == BODY_START_MARKER:
			state = BODY_START
		case state == BODY_START && input == '\n':
			state = BODY_READ_SAMPLE
			buff = []byte{}
			bytesLeftInSample = bytesPerSample
		case state == BODY_READ_SAMPLE:
			buff = append(buff, input)
			bytesLeftInSample = bytesLeftInSample - 1

			if bytesLeftInSample <= 0 {
				samplesLeftInSet = samplesLeftInSet - 1

				if samplesLeftInSet <= 0 {
					// Finish and send set
					bufWithHeader := append([]byte{DRIVER_PROTOCOL_VERSION}, buff...)
					onReceive(bufWithHeader)

					// Get ready for next set and request it
					state = WAITING_FOR_HEADER
					// Optional for Flex v4, mandatory for v5
					_, err = port.Write(START_MEASUREMENT_CMD)
					if err != nil {
						logger.WithField("error", err).Info("Failed to write poll message to serial port.")
						return
					}
				} else {
					// Start next point
					bytesLeftInSample = bytesPerSample
				}
			}
		case state == UNEXPECTED_BYTE && input == HEADER_START_MARKER:
			// Recover from error state when a new header is seen
			state = HEADER_START
		default:
			state = UNEXPECTED_BYTE
		}
	}
}
