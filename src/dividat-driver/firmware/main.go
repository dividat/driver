package firmware

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/pin/tftp"
)

const tftpPort = "69"
const controllerPort = "55567"

// Command-line interface to Update
func Command(flags []string) {
	updateFlags := flag.NewFlagSet("update", flag.ExitOnError)
	imagePath := updateFlags.String("i", "", "Firmware image path")
	configuredAddr := updateFlags.String("a", "", "Senso address (optional)")
	sensoSerial := updateFlags.String("s", "", "Senso serial (optional)")
	updateFlags.Parse(flags)

	var deviceSerial *string = nil
	if *sensoSerial != "" {
		deviceSerial = sensoSerial
	}

	if *imagePath == "" {
		flag.PrintDefaults()
		return
	}
	file, err := os.Open(*imagePath)
	if err != nil {
		fmt.Printf("Could not open image file: %v\n", err)
		os.Exit(1)
	}

	onProgress := func(progressMsg string) {
		fmt.Println(progressMsg)
	}

	Update(context.Background(), file, deviceSerial, configuredAddr, onProgress)

	if err != nil {
		fmt.Println(err.Error())
		fmt.Println()
		fmt.Println("Update failed. Try turning the Senso off and on, waiting for 30 seconds and then running this update tool again.")
		os.Exit(1)
	}
}

type OnProgress func(msg string)

// Firmware update workhorse
func Update(parentCtx context.Context, image io.Reader, deviceSerial *string, configuredAddr *string, onProgress OnProgress) (fail error) {
	// 1: Find address of a Senso in normal mode
	var controllerHost string
	if *configuredAddr != "" {
		// Use specified controller address
		controllerHost = *configuredAddr
		onProgress(fmt.Sprintf("Using specified controller address '%s'.", controllerHost))
	} else {
		// Discover controller address via mDNS
		ctx, cancel := context.WithTimeout(parentCtx, 15*time.Second)
		discoveredAddr, err := discover("_sensoControl._tcp", deviceSerial, ctx, onProgress)
		cancel()
		if err != nil {
			onProgress(fmt.Sprintf("Error: %s", err))
		} else {
			controllerHost = discoveredAddr
		}
	}

	// 2: Switch the Senso to bootloader mode
	if controllerHost != "" {
		err := sendDfuCommand(controllerHost, controllerPort, onProgress)
		if err != nil {
			// Log the failure, but continue anyway, as the Senso might have been left in
			// bootloader mode when a previous update process failed. Not all versions of
			// the firmware automtically exit from the bootloader mode upon restart.
			onProgress(fmt.Sprintf("Could not send DFU command to Senso at %s: %s", controllerHost, err))
		}
	} else {
		onProgress("Could not discover a Senso in regular mode, now trying to detect a Senso already in bootloader mode.")
	}

	// 3: Find address of Senso in bootloader mode
	var dfuHost string
	if *configuredAddr != "" {
		dfuHost = *configuredAddr
	} else {
		ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
		discoveredAddr, err := discover("_sensoUpdate._udp", deviceSerial, ctx, onProgress)
		cancel()
		if err != nil {
			// Up to firmware 2.0.0.0 the bootloader advertised itself with the same
			// service identifier as the application level firmware. To support such
			// legacy devices, we look for `_sensoControl` again at this point, if
			// the other service has not been found.
			// We do need to rediscover, as the legacy device may still just have
			// restarted into the bootloader and obtained a new IP address.
			ctx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
			legacyDiscoveredAddr, err := discover("_sensoControl._tcp", deviceSerial, ctx, onProgress)
			cancel()
			if err == nil {
				dfuHost = legacyDiscoveredAddr
			} else if controllerHost != "" {
				onProgress(fmt.Sprintf("Could not discover update service, trying to fall back to previous discovery %s.", controllerHost))
				dfuHost = controllerHost
			} else {
				msg := fmt.Sprintf("Could not find any Senso bootloader to transmit firmware to: %s", err)
				onProgress(msg)
				fail = fmt.Errorf(msg)
				return
			}
		} else {
			dfuHost = discoveredAddr
		}
	}

	// 4: Transmit firmware via TFTP
	time.Sleep(5 * time.Second) // Wait to ensure proper TFTP startup
	err := putTFTP(dfuHost, tftpPort, image, onProgress)
	if err != nil {
		fail = err
		return
	}

	onProgress("Success! Firmware transmitted to Senso.")
	return
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

	onProgress(fmt.Sprintf("Sent DFU command to %s:%s.", host, port))

	return nil
}

func putTFTP(host string, port string, image io.Reader, onProgress OnProgress) error {
	client, err := tftp.NewClient(fmt.Sprintf("%s:%s", host, port))
	if err != nil {
		return fmt.Errorf("Could not create tftp client: %v", err)
	}
	rf, err := client.Send("controller-app.bin", "octet")
	if err != nil {
		return fmt.Errorf("Could not create send connection: %v", err)
	}
	n, err := rf.ReadFrom(image)
	if err != nil {
		return fmt.Errorf("Could not read from file: %v", err)
	}
	onProgress(fmt.Sprintf("%d bytes sent", n))
	return nil
}

func discover(service string, deviceSerial *string, ctx context.Context, onProgress OnProgress) (addr string, err error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		err = fmt.Errorf("Initializing discovery failed: %v", err)
		return
	}

	onProgress(fmt.Sprintf("Starting discovery: %s", service))

	entries := make(chan *zeroconf.ServiceEntry)

	err = resolver.Browse(ctx, service, "local.", entries)
	if err != nil {
		err = fmt.Errorf("Browsing failed: %v", err)
		return
	}

	devices := make(map[string][]string)
	entriesWithoutSerial := 0
	select {
	case entry := <-entries:
		if entry == nil {
			break
		}

		var serial string
		for ix, txt := range entry.Text {
			if strings.HasPrefix(txt, "ser_no=") {
				serial = cleanSerial(strings.TrimPrefix(txt, "ser_no="))
				break
			} else if ix == len(entry.Text)-1 {
				entriesWithoutSerial++
				serial = fmt.Sprintf("UNKNOWN-%d", entriesWithoutSerial)
			}
		}
		if deviceSerial != nil && serial != *deviceSerial {
			break
		}
		for _, addrCandidate := range entry.AddrIPv4 {
			if addrCandidate.String() == "0.0.0.0" {
				onProgress(fmt.Sprintf("Skipping discovered address 0.0.0.0 for %s.", serial))
			} else {
				devices[serial] = append(devices[serial], addrCandidate.String())
			}
		}
	}

	if len(devices) == 0 && deviceSerial == nil {
		err = fmt.Errorf("Could not find any devices for service %s.", service)
	} else if len(devices) == 0 && deviceSerial != nil {
		err = fmt.Errorf("Could not find Senso %s.", *deviceSerial)
	} else if len(devices) == 1 {
		for serial, addrs := range devices {
			addr = addrs[0]
			onProgress(fmt.Sprintf("Discovered %s at %v, using %s.", serial, addrs, addr))
			return
		}
	} else {
		err = fmt.Errorf("Discovered multiple Sensos: %v. Please specify a serial or IP.", devices)
		return
	}
	return
}

func cleanSerial(serialStr string) string {
	// Senso firmware up to 3.8.0 adds garbage at end of serial in mDNS
	// entries due to improper string sizing.  Because bootloader firmware
	// will not be updated via Ethernet, the problem will stay around for a
	// while and we clean up the serial here to produce readable output for
	// older devices.
	return strings.Split(serialStr, "\\000")[0]
}
