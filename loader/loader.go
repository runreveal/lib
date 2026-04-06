package loader

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"sync"

	"github.com/segmentio/encoding/json"
	"github.com/tailscale/hujson"
	"github.com/tidwall/gjson"
)

// LoadConfig loads a configuration from a byte slice into the given config.
// The config must be a pointer to a struct.
// It parses the byteslice as hujson, which allows for C-style comments and
// trailing commas on arrays and maps.
// It replaces any environment variables in the JSON string before unmarshalling,
// properly escaping values to maintain JSON structure.
// Finally, it unmarshals the JSON into the config struct.
func LoadConfig(bts []byte, cfg any) error {
	bts, err := hujson.Standardize(bts)
	if err != nil {
		return err
	}
	bts = replaceEnvInJSON(bts)
	err = json.Unmarshal(bts, cfg)
	if err != nil {
		return err
	}
	return nil
}

type Builder[T any] interface {
	Configure() (T, error)
}

type Registry[T any] struct {
	m map[string]func() Builder[T]
	sync.RWMutex
}

var registry = struct {
	// map[typeString]registry[T] where T is variadic and typeString is:
	// reflect.TypeOf(T).String()
	m map[string]any
	sync.RWMutex
}{
	m: make(map[string]any),
}

func loadTypeReg(typ string) (any, error) {
	registry.RLock()
	defer registry.RUnlock()
	factory, ok := registry.m[typ]
	if !ok {
		return nil, fmt.Errorf("tried to unmarshal unregistered type: %s", typ)
	}
	return factory, nil
}

// Register registers a factory method for a type T with the given type name. T
// is typically an interface that is implmented by the struct of type given by
// the name.
func Register[T any](name string, factory func() Builder[T]) {
	registry.Lock()
	defer registry.Unlock()

	// get the go string representation of the type T
	// this is used to lookup the appropriate factory methods later
	typ := new(T)
	typStr := reflect.TypeOf(typ).String()
	typReg, ok := registry.m[typStr]
	if !ok {
		typReg = &Registry[T]{
			m: make(map[string]func() Builder[T]),
		}
		registry.m[typStr] = typReg
	}
	registryForType := typReg.(*Registry[T])
	registryForType.m[name] = factory
}

// List returns the sorted names of all registered implementations for type T.
func List[T any]() []string {
	typ := new(T)
	typStr := reflect.TypeOf(typ).String()

	registry.RLock()
	defer registry.RUnlock()

	raw, ok := registry.m[typStr]
	if !ok {
		return nil
	}
	reg := raw.(*Registry[T])
	reg.RLock()
	defer reg.RUnlock()

	names := make([]string, 0, len(reg.m))
	for name := range reg.m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Describe returns a zero-value Builder for the named type, useful for
// introspecting config fields (e.g. reflecting on JSON struct tags for
// help output). Returns nil, false if the name is not registered.
func Describe[T any](name string) (Builder[T], bool) {
	typ := new(T)
	typStr := reflect.TypeOf(typ).String()

	registry.RLock()
	raw, ok := registry.m[typStr]
	registry.RUnlock()
	if !ok {
		return nil, false
	}
	reg := raw.(*Registry[T])
	reg.RLock()
	factory, ok := reg.m[name]
	reg.RUnlock()
	if !ok {
		return nil, false
	}
	return factory(), true
}

// Loader is a struct which can dyanmically unmarshal any type T
type Loader[T any] struct {
	Builder[T]
}

func (b *Loader[T]) UnmarshalJSON(raw []byte) error {
	typ := new(T)
	typStr := reflect.TypeOf(typ).String()
	typReg, err := loadTypeReg(typStr)
	if err != nil {
		return err
	}
	registryForType := typReg.(*Registry[T])

	loadType := gjson.Get(string(raw), "type")
	if !loadType.Exists() {
		return fmt.Errorf("failed to unmarshal, missing type")
	}

	registryForType.RLock()
	factory, ok := registryForType.m[loadType.Str]
	registryForType.RUnlock()
	if !ok {
		return fmt.Errorf("failed to unmarshal, unknown type: %s", loadType.Str)
	}
	b.Builder = factory()
	return json.Unmarshal(raw, b.Builder)
}

func (l Loader[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.Builder)
}

func (l Loader[T]) Configure() (T, error) {
	var t T
	if l.Builder == nil {
		return t, errors.New("no type registered for configuration")
	}
	return l.Builder.Configure()
}

var envVarRegex = regexp.MustCompile(`"\$([A-Za-z_][A-Za-z0-9_]*)"`)

// replaceEnvInJSON replaces environment variables in JSON string values
// while properly escaping the replacement values to maintain JSON structure
func replaceEnvInJSON(jsonBytes []byte) []byte {
	return envVarRegex.ReplaceAllFunc(jsonBytes, func(match []byte) []byte {
		// Extract the environment variable name (without the $ prefix and quotes)
		envVar := string(match[2 : len(match)-1])
		envValue := os.Getenv(envVar)

		// Escape backslashes and quotes in the environment value
		escapedValue := escapeForJSON(envValue)

		// Return the escaped value wrapped in quotes
		return []byte(`"` + escapedValue + `"`)
	})
}

// escapeForJSON escapes a string for JSON using Go's json.Marshal
// This ensures RFC 7159/8259 compliance for all special characters
func escapeForJSON(s string) string {
	// Use json.Marshal to properly escape the string per RFC 7159/8259
	escaped, err := json.Marshal(s)
	if err != nil {
		// This should never happen for a string, but fallback just in case
		return s
	}
	// Remove the outer quotes that json.Marshal adds
	return string(escaped[1 : len(escaped)-1])
}
