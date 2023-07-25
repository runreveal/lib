package loader

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/segmentio/encoding/json"
	"github.com/tailscale/hujson"
	"github.com/tidwall/gjson"
)

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
	Configure() (T, error)
}

type Registry[T any] struct {
	m map[string]func() Builder[T]
	sync.RWMutex
}

var registry = struct {
	// map[type]registry[T] where T is variadic
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

func (l Loader[T]) Configure() (T, error) {
	return l.Builder.Configure()
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
