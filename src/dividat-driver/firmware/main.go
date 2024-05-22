package firmware

// Functions for performing a firmware update.
// The update procedure consists of the following high-level steps:
//
// 1. Discover Senso via mDNS
//
// 2. If the Senso is found to be in application mode,
//    send a DFU (Device Firmware Update) command
//    to make the Senso reboot into bootloader mode.
//
// 3. Transfer the firmware image via TFTP.

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/pin/tftp"

	"github.com/dividat/driver/src/dividat-driver/service"
)

const controllerPort = "55567"
const discoveryTimeout = 60 * time.Second

type OnProgress func(msg string)

func UpdateBySerial(ctx context.Context, deviceSerial string, image io.Reader, onProgress OnProgress) error {
	onProgress(fmt.Sprintf("Looking for Senso with specified serial %s", deviceSerial))
	match := service.Find(ctx, discoveryTimeout, service.SerialNumberFilter(deviceSerial))
	if match == nil {
		return fmt.Errorf("Failed to find Senso with serial number %s", deviceSerial)
	}

	onProgress(fmt.Sprintf("Found Senso at %s", match.Address))
	return update(ctx, *match, image, onProgress)
}

func update(parentCtx context.Context, target service.Service, image io.Reader, onProgress OnProgress) error {
	if !service.IsDfuService(target) {
		trySendDfu := func() error {
			err := sendDfuCommand(target.Address, controllerPort, onProgress)
			return err
		}

		backoffStrategy := backoff.NewExponentialBackOff()
		backoffStrategy.MaxElapsedTime = 30 * time.Second
		backoffStrategy.MaxInterval = 10 * time.Second
		err := backoff.RetryNotify(trySendDfu, backoffStrategy, func(e error, d time.Duration) {
			onProgress(fmt.Sprintf("%v\nRetrying in %v", e, d))
		})

		if err != nil {
			return fmt.Errorf("Could not send DFU command to Senso at %s: %s", target.Address, err)
		}

		onProgress("Looking for Senso in bootloader mode")
		dfuService := service.Find(parentCtx, discoveryTimeout, func(discovered service.Service) bool {
			return service.SerialNumberFilter(target.Text.Serial)(discovered) && service.IsDfuService(discovered)
		})

		if dfuService == nil {
			return fmt.Errorf("Could not find Senso in bootloader mode")
		}

		target = *dfuService
		onProgress(fmt.Sprintf("Found Senso in bootloader mode at %s", target.Address))
		onProgress("Waiting 10 seconds to ensure proper TFTP startup")
		// Wait to ensure proper TFTP startup
		time.Sleep(10 * time.Second)
	} else {
		onProgress("Found Senso in bootloader mode")
	}

	err := putTFTP(target.Address, strconv.Itoa(target.ServiceEntry.Port), image, onProgress)
	if err != nil {
		return err
	}

	return nil
}

func sendDfuCommand(host string, port string, onProgress OnProgress) error {
	// Header
	const PROTOCOL_VERSION = 0x00
	const NUMOFBLOCKS = 0x01
	reserve := bytes.Repeat([]byte{0x00}, 6)
	header := append([]byte{PROTOCOL_VERSION, NUMOFBLOCKS}, reserve...)

	// Message Body
	const BLOCKLENGTH = 0x0008
	const BLOCKTYPE_DFU = 0x00F0
	const MAGIC_KEY = 0xFA173CCD87664FBE
	body := make([]byte, 12)
	binary.LittleEndian.PutUint16(body[0:], BLOCKLENGTH)
	binary.LittleEndian.PutUint16(body[2:], BLOCKTYPE_DFU)
	binary.BigEndian.PutUint64(body[4:], MAGIC_KEY)

	command := append(header, body...)

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", host, port))
	if err != nil {
		return fmt.Errorf("Could not dial connection to Senso controller at %s:%s: %v", host, port, err)
	}
	defer conn.Close()
	time.Sleep(1 * time.Second)

	_, err = io.Copy(conn, bytes.NewReader(command))
	if err != nil {
		return fmt.Errorf("Could not send DFU command: %v", err)
	}

	onProgress(fmt.Sprintf("Sent DFU command to %s:%s", host, port))

	return nil
}

func putTFTP(host string, port string, image io.Reader, onProgress OnProgress) error {
	onProgress("Creating TFTP client")
	client, err := tftp.NewClient(fmt.Sprintf("%s:%s", host, port))
	if err != nil {
		return fmt.Errorf("Could not create tftp client: %v", err)
	}

	maxRetries := 5
	client.SetRetries(maxRetries)
	// It can take a while for the Senso to respond to the TFTP write request.
	// Setting timeout to 10 seconds prevents unnecessary messages about failed
	// send attempts.
	client.SetTimeout(10 * time.Second)

	expDelay := func(attempt int) time.Duration {
		exp := math.Pow(2, float64(attempt))
		exp = math.Min(exp, 60)
		return time.Duration(int(exp)) * time.Second
	}

	client.SetBackoff(func(attempt int) time.Duration {
		delay := expDelay(attempt)
		msg := fmt.Sprintf("Failed on attempt %d, retrying in %v", attempt+1, delay)
		onProgress(msg)
		return delay
	})

	onProgress("Preparing transmission")
	rf, err := client.Send("controller-app.bin", "octet")
	if err != nil {
		return fmt.Errorf("Could not create send connection: %v", err)
	}
	onProgress("Transmitting...")
	n, err := rf.ReadFrom(image)
	if err != nil {
		return fmt.Errorf("Could not read from file: %v", err)
	}
	onProgress(fmt.Sprintf("%d bytes sent", n))
	return nil
}

// State to keep track of when an update is in progress.
// This is used by the senso module, but is kept here to
// ensure privacy of internals.

type Update struct {
	stateMutex sync.Mutex
	inProgress bool
}

func InitialUpdateState() *Update {
	return &Update{
		inProgress: false,
		stateMutex: sync.Mutex{},
	}
}

func (u *Update) IsUpdating() bool {
	u.stateMutex.Lock()
	defer u.stateMutex.Unlock()
	return u.inProgress
}

func (u *Update) SetUpdating(state bool) {
	u.stateMutex.Lock()
	defer u.stateMutex.Unlock()
	u.inProgress = state
}
