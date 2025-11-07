package senso

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/cskr/pubsub"
	"github.com/sirupsen/logrus"

	"github.com/dividat/driver/src/dividat-driver/firmware"
	"github.com/dividat/driver/src/dividat-driver/service"
	"github.com/dividat/driver/src/dividat-driver/util/websocket"
)

// pubsub topic names, must be unique
const brokerTopicRx = "rx"
const brokerTopicTx = "tx"

// Handle for managing Senso
type Handle struct {
	websocket.Handle
}

type DeviceBackend struct {
	ctx context.Context
	log *logrus.Entry

	address        *string
	firmwareUpdate *firmware.Update

	broker *pubsub.PubSub

	cancelCurrentConnection context.CancelFunc
	connectionChangeMutex   *sync.Mutex
}

func (backend *DeviceBackend) RegisterSubscriber(r *http.Request) {
	// noop
	return
}

func (backend *DeviceBackend) Discover(duration int, ctx context.Context) chan websocket.DeviceInfo {
	discoveryCtx, _ := context.WithTimeout(ctx, time.Duration(duration)*time.Second)
	// map over the channel to wrap ServiceEntry into DeviceInfo
	deviceChan := make(chan websocket.DeviceInfo)
	go func() {
		for service := range service.Scan(discoveryCtx) {
			device := websocket.DeviceInfo{TcpDeviceInfo: &service.ServiceEntry}
			deviceChan <- device
		}
		close(deviceChan)
	}()
	return deviceChan
}

func (backend *DeviceBackend) GetStatus() websocket.Status {
	return websocket.Status{
		Address: backend.address,
	}
}

func (backend *DeviceBackend) IsUpdatingFirmware() bool {
	return backend.firmwareUpdate.IsUpdating()
}

// New returns an initialized Senso handler
func New(ctx context.Context, log *logrus.Entry) *Handle {
	backend := DeviceBackend{
		ctx: ctx,
		log: log,

		broker: pubsub.New(32),

		connectionChangeMutex: &sync.Mutex{},
		firmwareUpdate:        firmware.InitialUpdateState(),
	}

	websocketHandle := websocket.Handle{
		DeviceBackend: &backend,
		Broker:        backend.broker,
		BrokerRx:      brokerTopicRx,
		BrokerTx:      brokerTopicTx,
		Log:           log,
	}
	handle := Handle{Handle: websocketHandle}

	// Clean up
	go func() {
		<-ctx.Done()
		backend.broker.Shutdown()
	}()

	return &handle
}

func (backend *DeviceBackend) DeregisterSubscriber() {
	// noop
}

// Connect to a Senso, will create TCP connections to control and data ports
func (backend *DeviceBackend) Connect(address string) {

	// Only allow one connection change at a time
	backend.connectionChangeMutex.Lock()
	defer backend.connectionChangeMutex.Unlock()

	// disconnect current connection first
	backend.Disconnect()

	// set address in backend
	backend.address = &address

	// Create a child context for a new connection. This allows an individual connection (attempt) to be cancelled without restarting the whole Senso backendr
	ctx, cancel := context.WithCancel(backend.ctx)

	backend.log.WithField("address", address).Info("Attempting to connect with Senso.")

	onReceive := func(data []byte) {
		backend.broker.TryPub(data, brokerTopicRx)
	}

	// TODO: noTx??
	go connectTCP(ctx, backend.log.WithField("channel", "data"), address+":55568", backend.broker.Sub("noTx"), onReceive)
	time.Sleep(1000 * time.Millisecond)
	go connectTCP(ctx, backend.log.WithField("channel", "control"), address+":55567", backend.broker.Sub(brokerTopicTx), onReceive)

	backend.cancelCurrentConnection = cancel
}

// Disconnect from current connection
func (backend *DeviceBackend) Disconnect() {
	if backend.cancelCurrentConnection != nil {
		backend.log.Info("Disconnecting from Senso.")
		backend.cancelCurrentConnection()
		backend.address = nil
	}
}
