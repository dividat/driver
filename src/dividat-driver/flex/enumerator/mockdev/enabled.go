//go:build debug

package mockdev

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	serialenum "go.bug.st/serial/enumerator"
)

type MockDeviceId int

var ErrDeviceNotFound = errors.New("mock device id not found")
var ErrDeviceExists = errors.New("mock device id is already defined")

type MockDeviceRegistry struct {
	log                   *logrus.Entry
	registeredMockDevices map[MockDeviceId]*serialenum.PortDetails
}

func New(log *logrus.Entry) *MockDeviceRegistry {
	log.Info("Mock device registry enabled (debug build)")
	return &MockDeviceRegistry{
		log:                   log,
		registeredMockDevices: make(map[MockDeviceId]*serialenum.PortDetails),
	}
}

func (h *MockDeviceRegistry) handlePost(w http.ResponseWriter, r *http.Request) {
	var portDetails serialenum.PortDetails
	if err := json.NewDecoder(r.Body).Decode(&portDetails); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	mockDeviceId := h.registerMockDevice(portDetails)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"id": int(mockDeviceId)})
}

func (h *MockDeviceRegistry) handleDelete(w http.ResponseWriter, r *http.Request) {
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

	if err := h.unregisterMockDevice(MockDeviceId(id)); err != nil {
		if err == ErrDeviceNotFound {
			http.Error(w, "Device not found", http.StatusNotFound)
		} else {
			http.Error(w, "Internal error", http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *MockDeviceRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handlePost(w, r)
		return
	case http.MethodDelete:
		h.handleDelete(w, r)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (h *MockDeviceRegistry) ListMockDevices() []*serialenum.PortDetails {
	values := slices.Collect(maps.Values(h.registeredMockDevices))
	return values
}

func (h *MockDeviceRegistry) nextMockDeviceId() MockDeviceId {
	maxId := MockDeviceId(-1)
	for id := range h.registeredMockDevices {
		if id > maxId {
			maxId = id
		}
	}
	return maxId + 1
}

func (h *MockDeviceRegistry) registerMockDevice(portDetails serialenum.PortDetails) MockDeviceId {
	mockDeviceId := h.nextMockDeviceId()
	h.registeredMockDevices[mockDeviceId] = &portDetails

	return mockDeviceId
}

func (h *MockDeviceRegistry) unregisterMockDevice(mockDeviceId MockDeviceId) error {
	if _, ok := h.registeredMockDevices[mockDeviceId]; !ok {
		return ErrDeviceNotFound
	}
	delete(h.registeredMockDevices, mockDeviceId)
	return nil
}
