package cli

import "sync"

var (
	registryMu sync.Mutex
	registry   []Node
)

// Register appends nodes to the package-level command registry.
// Intended for use in init() functions to enable build-tag-based
// inclusion of optional commands.
func Register(nodes ...Node) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = append(registry, nodes...)
}

// Registered returns a copy of all nodes added via Register.
// Call this when building the App to include registered commands.
func Registered() []Node {
	registryMu.Lock()
	defer registryMu.Unlock()
	out := make([]Node, len(registry))
	copy(out, registry)
	return out
}
