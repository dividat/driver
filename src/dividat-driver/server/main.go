package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/dividat/driver/src/dividat-driver/flex"
	flex_enumerator "github.com/dividat/driver/src/dividat-driver/flex/enumerator"
	"github.com/dividat/driver/src/dividat-driver/logging"
	"github.com/dividat/driver/src/dividat-driver/rfid"
	"github.com/dividat/driver/src/dividat-driver/senso"
)

// Uncomment following line for profiling. And run `go tool pprof http://localhost:8382/debug/pprof/profile` or `go tool pprof http://localhost:8382/debug/pprof/heap`
// import _ "net/http/pprof"

// build var (-ldflags)
var version string

const serverPort = "8382"

// Start the driver server
func Start(logger *logrus.Logger, origins []string) context.CancelFunc {
	// Log Server
	logServer := logging.NewLogServer()
	logger.AddHook(logServer)

	baseLog := logger.WithFields(logrus.Fields{
		"version": version,
	})

	// Get System information
	systemInfo, err := GetSystemInfo()
	if err != nil {
		baseLog.WithError(err).Panic("Could not get system information.")
	}

	baseLog = baseLog.WithFields(logrus.Fields{
		"machineId": systemInfo.MachineId,
		"os":        systemInfo.Os,
		"arch":      systemInfo.Arch,
	})

	baseLog.Info("Dividat Driver starting")

	// Setup log endpoint
	http.Handle("/log", originMiddleware(origins, baseLog, logServer))

	// Setup a context
	ctx, cancel := context.WithCancel(context.Background())

	// Setup Senso
	sensoHandle := senso.New(ctx, baseLog.WithField("package", "senso"))
	http.Handle("/senso", originMiddleware(origins, baseLog, sensoHandle))

	// Setup Flex reader
	flexEnumerator := flex_enumerator.New(ctx, baseLog.WithField("package", "flex.enumerator"))
	flexHandle := flex.New(ctx, baseLog.WithField("package", "flex"), flexEnumerator)
	http.Handle("/flex", originMiddleware(origins, baseLog, flexHandle))

	// Setup RFID scanner
	rfidHandle := rfid.NewHandle(ctx, baseLog.WithField("package", "rfid"))
	// net/http performs a redirect from `/rfid` if only `/rfid/` is mounted
	http.Handle("/rfid", originMiddleware(origins, baseLog, rfidHandle))
	http.Handle("/rfid/", originMiddleware(origins, baseLog, rfidHandle))

	// Create a logger for server
	log := baseLog.WithField("package", "server")

	// Start the monitor
	go startMonitor(baseLog.WithField("package", "monitor"))

	// Setup HTTP Server
	server := http.Server{Addr: "127.0.0.1:" + serverPort}

	// Server root
	rootMsg, _ := json.Marshal(map[string]string{
		"message":   "Dividat Driver",
		"version":   version,
		"machineId": systemInfo.MachineId,
		"os":        systemInfo.Os,
		"arch":      systemInfo.Arch,
	})
	http.Handle("/", originMiddleware(origins, baseLog, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(rootMsg)
	})))

	// Start the server
	log.WithField("port", serverPort).Info("Starting HTTP server.")

	go func() {
		serverErr := server.ListenAndServe()
		if serverErr != http.ErrServerClosed {
			log.Panic(serverErr)
		}
	}()

	// cleanup routine
	go func() {
		<-ctx.Done()

		log.Info("Server closing down.")
		server.Close()

	}()

	return cancel
}

// Middleware to ensure browser requests come from permissible origins.
//
// This protects anyone running the driver from malicious websites connecting
// to the loopback address. In order to protect WebSocket endpoints, for which
// CORS pre-flight requests are not performed, we fully deny requests from
// unknown origins instead of just withholding CORS headers.
func originMiddleware(origins []string, log *logrus.Entry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check whether a request was made from a permissible origin.
		// An absent Origin header indicates a non-browser request and is permissible.
		if origin != "" && !contains(origins, origin) {
			log.WithField("origin", r.Header.Get("Origin")).Info("Denying request from untrusted origin.")
			w.WriteHeader(403)
			return
		}

		// Set CORS/Private Network Access headers
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}

		// Announce that `Origin` header value may affect response
		w.Header().Set("Vary", "Origin")

		// Greenlight pre-flight requests, forward all other requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(200)
			return
		} else {
			next.ServeHTTP(w, r)
		}
	})
}

func contains(slice []string, candidate string) bool {
	for _, member := range slice {
		if member == candidate {
			return true
		}
	}
	return false
}
