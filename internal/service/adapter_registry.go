package service

import (
	"fmt"

	"github.com/user/notification-hub/internal/adapters"
)

type AdapterRegistry struct {
	adapters map[string]adapters.Adapter
}

func NewAdapterRegistry() *AdapterRegistry {
	return &AdapterRegistry{adapters: map[string]adapters.Adapter{}}
}

func (r *AdapterRegistry) Register(adapter adapters.Adapter) {
	if adapter == nil {
		return
	}
	r.adapters[adapter.Platform()] = adapter
}

func (r *AdapterRegistry) Get(platform string) (adapters.Adapter, error) {
	adapter, ok := r.adapters[platform]
	if !ok {
		return nil, fmt.Errorf("adapter not registered for platform %s", platform)
	}
	return adapter, nil
}
