package loader

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/segmentio/encoding/json"
	"github.com/tailscale/hujson"
	"github.com/tidwall/gjson"
)

// LoadConfig loads a configuration from a byte slice into the given config.
// The config must be a pointer to a struct.
// It parses the byteslice as hujson, which allows for C-style comments and
// trailing commas on arrays and maps.
// It then unmarshals the JSON into the config struct.
// Finally, it replaces any environment variables in the struct with their
// values referenced by the corresponding environment variables.
func LoadConfig(bts []byte, cfg any) error {
	bts, err := hujson.Standardize(bts)
	if err != nil {
		return err
	}
	err = json.Unmarshal(bts, cfg)
	if err != nil {
		return err
	}
	replaceEnv(reflect.ValueOf(cfg))
	return nil
}

type Builder[T any] interface {
	Configure(...Option) (T, error)
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

type Option interface {
	Apply()
}

func (l Loader[T]) Configure(opts ...Option) (T, error) {
	var t T
	if l.Builder == nil {
		return t, errors.New("no type registered for configuration")
	}
	return l.Builder.Configure(opts...)
}

func replaceEnv(v reflect.Value) {
	if !v.IsValid() {
		return
	}

	switch v.Kind() {
	case reflect.String:
		val := v.String()
		if v.CanSet() && strings.HasPrefix(val, "$") {
			envVar, _ := strings.CutPrefix(val, "$")
			v.SetString(os.Getenv(envVar))
		}
	case reflect.Ptr:
		replaceEnv(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			replaceEnv(v.Field(i))
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			replaceEnv(v.Index(i))
		}
	case reflect.Interface:
		if v.IsNil() {
			return
		}
		copied := reflect.New(v.Elem().Type()).Elem()
		copied.Set(v.Elem())
		replaceEnv(copied)
		v.Set(copied)
	}
}
