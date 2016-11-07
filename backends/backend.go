package backends

import (
	"fmt"

	guerrilla "github.com/flashmob/go-guerrilla"
)

var backends = map[string]guerrilla.Backend{}

// New retrive a backend specified by the backendName, and initialize it using
// backendConfig
func New(backendName string, backendConfig guerrilla.BackendConfig) (guerrilla.Backend, error) {
	backend, found := backends[backendName]
	if !found {
		return nil, fmt.Errorf("backend %q not found", backendName)
	}
	err := backend.Initialize(backendConfig)
	if err != nil {
		return nil, fmt.Errorf("error while initializing the backend: %s", err)
	}
	return backend, nil
}
