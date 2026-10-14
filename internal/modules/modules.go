// Package modules defines the contract for features installed into the proxy.
// Modules are compiled Go packages, registered in the order selected at startup.
package modules

import (
	"fmt"
	"strings"

	"github.com/elazarl/goproxy"
)

type Module interface {
	Name() string
	Register(*goproxy.ProxyHttpServer)
}

// Evidence lets modules record decisions and body samples without knowing how
// the application stores captures. Recording failures must not change traffic.
type Evidence interface {
	Record(map[string]interface{})
	SaveArtifact(name string, data []byte) error
}

// Select resolves a comma-separated list before any handlers are installed.
// An empty list enables no modules. Duplicate and unknown names are errors.
func Select(selection string, available ...Module) ([]Module, error) {
	byName := make(map[string]Module, len(available))
	for _, m := range available {
		if _, exists := byName[m.Name()]; exists {
			return nil, fmt.Errorf("duplicate available module %q", m.Name())
		}
		byName[m.Name()] = m
	}
	if strings.TrimSpace(selection) == "" {
		return nil, nil
	}
	var selected []Module
	seen := make(map[string]bool)
	for _, name := range strings.Split(selection, ",") {
		name = strings.TrimSpace(name)
		m, exists := byName[name]
		if !exists {
			return nil, fmt.Errorf("unknown module %q", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("module %q selected more than once", name)
		}
		seen[name] = true
		selected = append(selected, m)
	}
	return selected, nil
}
