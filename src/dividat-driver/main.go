package main

// Start up driver as a service

import (
	"context"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/dividat/driver/src/dividat-driver/firmware"
	"github.com/dividat/driver/src/dividat-driver/logging"
	"github.com/dividat/driver/src/dividat-driver/server"
	"github.com/kardianos/service"
	"github.com/sirupsen/logrus"
)

type program struct {
	close context.CancelFunc
}

func main() {
	// Serve command or start in daemon mode by default
	if len(os.Args) > 1 && os.Args[1] == "update-firmware" {
		firmware.Command(os.Args[2:])
	} else {
		runDaemon()
	}
}

func (p *program) Start(s service.Service) error {
	// Set up logging
	logger := logrus.New()
	if !service.Interactive() {
		logger.Out = ioutil.Discard
		if systemLogger, err := s.SystemLogger(nil); err == nil {
			logger.AddHook(logging.NewSystemHook(systemLogger))
		}
	}
	logger.SetLevel(logrus.DebugLevel)

	// Command-line flags
	var permissibleOrigins stringList
	flag.Var(&permissibleOrigins, "permissible-origin", "Permissible origin to make requests to the driver's HTTP endpoints, may be repeated. Default is a list of common Dividat origins.")

	testMode := flag.Bool("test-mode", false, "Run in test mode. Enables mock serial (Flex) device registration.")
	flag.Parse()

	if len(permissibleOrigins) == 0 {
		permissibleOrigins = defaultOrigins
	}

	if *testMode {
		logger.Info("Running in test mode!")
	}

	// Start server
	p.close = server.Start(logger, permissibleOrigins, *testMode)
	return nil
}

func (p *program) Stop(s service.Service) error {
	p.close()
	return nil
}

func runDaemon() {
	svcConfig := &service.Config{
		Name:        "DividatDriver",
		DisplayName: "Dividat Driver",
		Description: "Dividat Driver application for hardware connectivity.",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(s.Run())
}

// Flags

type stringList []string

func (list *stringList) String() string {
	return strings.Join(*list, ", ")
}

func (i *stringList) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var defaultOrigins []string = []string{
	"http://localhost:8080",
	"https://play.dividat.ch",
	"https://play.dividat.com",
	"https://val-play.dividat.ch",
	"https://val-play.dividat.com",
	"https://dev-play.dividat.ch",
	"https://dev-play.dividat.com",
	"https://lab.dividat.ch",
	"https://lab.dividat.com",
	"https://shed.dividat.ch",
	"https://shed.dividat.com",
}
