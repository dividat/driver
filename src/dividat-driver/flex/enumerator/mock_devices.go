package enumerator

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	serial_enumerator "go.bug.st/serial/enumerator"
)

type MockDeviceId int

var ErrDeviceNotFound = errors.New("mock device id not found")
var ErrDeviceExists = errors.New("mock device id is already defined")

func (handle *DeviceEnumerator) handlePost(w http.ResponseWriter, r *http.Request) {
	var portDetails serial_enumerator.PortDetails
	if err := json.NewDecoder(r.Body).Decode(&portDetails); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	mockDeviceId := handle.registerMockDevice(portDetails)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"id": int(mockDeviceId)})
}

func (handle *DeviceEnumerator) handleDelete(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) != 2 || pathParts[1] == "" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	idStr := pathParts[1]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := handle.unregisterMockDevice(MockDeviceId(id)); err != nil {
		if err == ErrDeviceNotFound {
			http.Error(w, "Device not found", http.StatusNotFound)
		} else {
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (handle *DeviceEnumerator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if handle.testMode {
		switch r.Method {
		case http.MethodPost:
			handle.handlePost(w, r)
			return
		case http.MethodDelete:
			handle.handleDelete(w, r)
			return
		}
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (handle *DeviceEnumerator) listMockDevices() []*serial_enumerator.PortDetails {
	// TODO: use map.Values after upgrading go >= 1.23
	ports := make([]*serial_enumerator.PortDetails, 0, len(handle.registeredMockDevices))
	for _, port := range handle.registeredMockDevices {
		ports = append(ports, port)
	}
	return ports
}

func (handle *DeviceEnumerator) nextMockDeviceId() MockDeviceId {
	maxId := MockDeviceId(-1)
	for id := range handle.registeredMockDevices {
		if id > maxId {
			maxId = id
		}
	}
	return maxId + 1
}

func (handle *DeviceEnumerator) registerMockDevice(portDetails serial_enumerator.PortDetails) MockDeviceId {
	mockDeviceId := handle.nextMockDeviceId()
	handle.registeredMockDevices[mockDeviceId] = &portDetails

	return mockDeviceId
}

func (handle *DeviceEnumerator) unregisterMockDevice(mockDeviceId MockDeviceId) error {
	if _, ok := handle.registeredMockDevices[mockDeviceId]; !ok {
		return ErrDeviceNotFound
	}
	delete(handle.registeredMockDevices, mockDeviceId)
	return nil
}
