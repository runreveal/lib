package cli

import (
	"fmt"
	"reflect"
	"strings"
)

// fieldInfo holds parsed tag info for a struct field.
type fieldInfo struct {
	flagLong   string
	flagShort  string
	configKey  string
	usage      string
	defVal     string
	fieldIndex []int // nested index path for embedded structs
	fieldType  reflect.Type
}

// scanFields walks a struct type and extracts all field metadata.
// It handles embedded structs by recursively scanning them.
func scanFields(t reflect.Type) ([]fieldInfo, error) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %s", t.Kind())
	}
	return scanFieldsWithIndex(t, nil)
}

func scanFieldsWithIndex(t reflect.Type, prefix []int) ([]fieldInfo, error) {
	var fields []fieldInfo
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		// Copy prefix into a new slice before appending i to avoid
		// aliasing the backing array across loop iterations.
		idx := append(append([]int{}, prefix...), i)

		// Handle embedded structs (anonymous fields)
		ft := f.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if f.Anonymous && ft.Kind() == reflect.Struct {
			sub, err := scanFieldsWithIndex(ft, idx)
			if err != nil {
				return nil, err
			}
			fields = append(fields, sub...)
			continue
		}

		cliTag := f.Tag.Get("cli")
		configTag := f.Tag.Get("config")
		usage := f.Tag.Get("usage")
		defVal := f.Tag.Get("default")

		// Skip if no relevant tags at all
		if cliTag == "" && configTag == "" {
			continue
		}

		// cli:"-" means skip
		if cliTag == "-" {
			continue
		}

		info := fieldInfo{
			configKey:  configTag,
			usage:      usage,
			defVal:     defVal,
			fieldIndex: idx,
			fieldType:  f.Type,
		}

		if cliTag != "" {
			parts := strings.SplitN(cliTag, ",", 2)
			info.flagLong = parts[0]
			if len(parts) == 2 {
				info.flagShort = strings.TrimSpace(parts[1])
			}
		}

		fields = append(fields, info)
	}
	return fields, nil
}

// buildFlagSet constructs a FlagSet by reflecting on the handler's struct tags.
// It also returns the scanned fields so callers can reuse them (e.g. for
// applyDefaults and config loading) without re-scanning.
func buildFlagSet(handler Runnable) (*FlagSet, []fieldInfo, error) {
	rv := reflect.ValueOf(handler)
	if rv.Kind() != reflect.Ptr {
		return nil, nil, fmt.Errorf("handler must be a pointer to a struct")
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("handler must be a pointer to a struct")
	}

	fields, err := scanFields(rv.Type())
	if err != nil {
		return nil, nil, err
	}

	fs := newFlagSet()
	for _, fi := range fields {
		if fi.flagLong == "" {
			continue
		}

		fieldVal := fieldByIndex(rv, fi.fieldIndex)
		ptr := fieldVal.Addr().Interface()

		def, err := makeFlagDef(fi.flagLong, fi.flagShort, fi.usage, fi.defVal, ptr)
		if err != nil {
			return nil, nil, fmt.Errorf("field %v: %w", fi.fieldIndex, err)
		}

		if err := fs.add(def); err != nil {
			return nil, nil, err
		}
	}

	return fs, fields, nil
}

// addGlobalsToFlagSet scans a globals struct pointer and adds its cli-tagged
// fields to the given FlagSet. Returns the scanned fields for default application.
func addGlobalsToFlagSet(fs *FlagSet, globals any) ([]fieldInfo, error) {
	rv := reflect.ValueOf(globals)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("globals must be a pointer to a struct")
	}
	rv = rv.Elem()

	fields, err := scanFields(rv.Type())
	if err != nil {
		return nil, err
	}

	for _, fi := range fields {
		if fi.flagLong == "" {
			continue
		}
		// Skip if handler already defines this flag (handler wins).
		if _, exists := fs.byLong[fi.flagLong]; exists {
			continue
		}

		fieldVal := fieldByIndex(rv, fi.fieldIndex)
		ptr := fieldVal.Addr().Interface()

		def, err := makeFlagDef(fi.flagLong, fi.flagShort, fi.usage, fi.defVal, ptr)
		if err != nil {
			return nil, fmt.Errorf("globals field %v: %w", fi.fieldIndex, err)
		}
		if err := fs.add(def); err != nil {
			return nil, err
		}
	}

	return fields, nil
}

// applyDefaults sets the default values using pre-scanned fields.
func applyDefaults(fs *FlagSet, fields []fieldInfo) error {
	for _, fi := range fields {
		if fi.defVal == "" || fi.flagLong == "" {
			continue
		}
		def, ok := fs.byLong[fi.flagLong]
		if !ok {
			continue
		}
		if err := def.setValue(fi.defVal); err != nil {
			return fmt.Errorf("applying default %q to --%s: %w", fi.defVal, fi.flagLong, err)
		}
	}
	return nil
}

// fieldByIndex retrieves a nested struct field by index path, initializing
// embedded pointer structs as needed.
func fieldByIndex(v reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}
