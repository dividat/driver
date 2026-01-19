package senso

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/sirupsen/logrus"
)

// How long to wait before timeing out a tcp connection attempt
const dialTimeout = 5 * time.Second

// maximal interval to wait between connection retry
const maxInterval = 30 * time.Second

type onReceive = func([]byte)

// connectTCP creates a persistent tcp connection to address
// isOptional is used when connecting to the data channel, which is used only in legacy Senso firmware
func connectTCP(ctx context.Context, baseLogger *logrus.Entry, address string, tx chan interface{}, onReceive onReceive, isOptional bool) {
	var dialer net.Dialer

	var log = baseLogger.WithField("address", address)

	var connectionAttemptLogLevel logrus.Level
	if isOptional {
		connectionAttemptLogLevel = logrus.DebugLevel
	} else {
		connectionAttemptLogLevel = logrus.InfoLevel
	}

	var conn net.Conn
	dialTCP := func() error {

		dialer.Deadline = time.Now().Add(dialTimeout)
		var connErr error
		if conn != nil {
			conn.Close()
		}

		log.Log(connectionAttemptLogLevel, "Dialing TCP connection.")
		conn, connErr = dialer.DialContext(ctx, "tcp", address)

		if connErr != nil {
			log.WithError(connErr).Log(connectionAttemptLogLevel, "Could not connect with Senso.")
		}
		return connErr
	}

	// Exponential backoff
	var expBackoff = backoff.NewExponentialBackOff()
	// Never stop retrying
	expBackoff.MaxElapsedTime = 0
	// Set maximum interval to 30s
	expBackoff.MaxInterval = maxInterval

	var backOffStrategy = backoff.WithContext(expBackoff, ctx)

	defer log.Info("Connection closed.")

	for true {

		backOffStrategy.Reset()
		backoff.Retry(dialTCP, backOffStrategy)

		// connection/ctx has been cancelled
		if conn == nil {
			return
		}

		log.Info("Connected.")

		// Close connection if we break or return
		defer conn.Close()

		// create channel for reading data and go read
		readChannel := make(chan []byte)
		go tcpReader(log, conn, readChannel)

		// Inner loop for handling data
		disconnected := false
		for !disconnected {
			select {

			case <-ctx.Done():
				return

			case receivedData, more := <-readChannel:
				if more {
					// Attempt to send data, if can not send immediately discard
					onReceive(receivedData)
				} else {
					disconnected = true
					break
				}

			case i := <-tx:
				data, _ := i.([]byte)
				err := write(conn, data)
				if err != nil {
					disconnected = true
					break
				}
			}
		}

	}
}

// Helper to read from TCP connection
func tcpReader(log *logrus.Entry, conn net.Conn, channel chan<- []byte) {

	defer close(channel)

	buffer := make([]byte, 1024)

	// Loop and read from connection.
	for {
		readN, readErr := conn.Read(buffer)

		if readErr != nil {
			if readErr == io.EOF {
				return
			} else if err, ok := readErr.(net.Error); ok && err.Timeout() {
				// Read timeout, just continue Nothing
			} else {
				// log.WithError(readErr).Error("Read error.")
				return
			}
		} else {
			channel <- buffer[:readN]
		}
	}
}

func write(conn net.Conn, data []byte) error {
	if conn != nil {
		conn.SetWriteDeadline(time.Now().Add(1 * time.Millisecond))
		_, err := conn.Write(data)
		return err
	} else {
		return errors.New("Can not write to TCP connection, because not connected.")
	}
}
