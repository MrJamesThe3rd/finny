package document

import (
	"encoding/json"
	"fmt"
)

// BackendFactory creates a Backend from the JSONB config stored in document_backends.
type BackendFactory func(config json.RawMessage) (Backend, error)

// Registry maps backend type strings to their factories.
// Register backends at startup; the service instantiates them on demand.
type Registry struct {
	factories map[string]BackendFactory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]BackendFactory)}
}

func (r *Registry) Register(backendType string, factory BackendFactory) {
	r.factories[backendType] = factory
}

func (r *Registry) Create(backendType string, config json.RawMessage) (Backend, error) {
	factory, ok := r.factories[backendType]
	if !ok {
		return nil, fmt.Errorf("unknown backend type: %s", backendType)
	}

	return factory(config)
}
