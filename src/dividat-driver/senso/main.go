package senso

import (
	"context"
	"sync"
	"time"

	"github.com/cskr/pubsub"
	"github.com/sirupsen/logrus"

	"github.com/dividat/driver/src/dividat-driver/firmware"
)

// Handle for managing Senso
type Handle struct {
	broker *pubsub.PubSub

	Address *string

	ctx context.Context

	cancelCurrentConnection context.CancelFunc
	connectionChangeMutex   *sync.Mutex

	firmwareUpdate *firmware.Update

	log *logrus.Entry
}

// New returns an initialized Senso handler
func New(ctx context.Context, log *logrus.Entry) *Handle {
	handle := Handle{}

	handle.ctx = ctx

	handle.log = log

	handle.connectionChangeMutex = &sync.Mutex{}
	handle.firmwareUpdate = firmware.InitialUpdateState()

	// PubSub broker
	handle.broker = pubsub.New(32)

	// Clean up
	go func() {
		<-ctx.Done()
		handle.broker.Shutdown()
	}()

	return &handle
}

// Connect to a Senso, will create TCP connections to control and data ports
func (handle *Handle) Connect(address string) {

	// Only allow one connection change at a time
	handle.connectionChangeMutex.Lock()
	defer handle.connectionChangeMutex.Unlock()

	// disconnect current connection first
	handle.Disconnect()

	// set address in handle
	handle.Address = &address

	// Create a child context for a new connection. This allows an individual connection (attempt) to be cancelled without restarting the whole Senso handler
	ctx, cancel := context.WithCancel(handle.ctx)

	handle.log.WithField("address", address).Info("Attempting to connect with Senso.")

	onReceive := func(data []byte) {
		handle.broker.TryPub(data, "rx")
	}

	go connectTCP(ctx, handle.log.WithField("channel", "data"), address+":55568", handle.broker.Sub("noTx"), onReceive, true)
	time.Sleep(1000 * time.Millisecond)
	go connectTCP(ctx, handle.log.WithField("channel", "control"), address+":55567", handle.broker.Sub("tx"), onReceive, false)

	handle.cancelCurrentConnection = cancel
}

// Disconnect from current connection
func (handle *Handle) Disconnect() {
	if handle.cancelCurrentConnection != nil {
		handle.log.Info("Disconnecting from Senso.")
		handle.cancelCurrentConnection()
		handle.Address = nil
	}
}
