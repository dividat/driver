//go:build !debug

package mockdev

import (
	"net/http"

	"github.com/sirupsen/logrus"
	serialenum "go.bug.st/serial/enumerator"
)

type MockDeviceRegistry struct {
}

func New(log *logrus.Entry) *MockDeviceRegistry {
	return &MockDeviceRegistry{}
}

func (h *MockDeviceRegistry) ListMockDevices() []*serialenum.PortDetails {
	return nil
}

// Mock device registration is only available in debug builds.
func (h *MockDeviceRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Mock device registration is not available in production builds", http.StatusForbidden)
}
