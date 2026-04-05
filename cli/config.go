package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"

	"github.com/runreveal/lib/loader"
	"github.com/tidwall/gjson"
)

// loadConfigIntoHandler reads the config file (path from the named flag),
// processes it via loader.LoadConfig, then applies config-tagged fields.
// Only sets fields that were NOT explicitly set via CLI flags.
// fields must be pre-scanned via buildFlagSet to avoid redundant reflection.
func loadConfigIntoHandler(handler Runnable, fs *FlagSet, configFlagName string, fields []fieldInfo) error {
	if _, ok := fs.byLong[configFlagName]; !ok {
		return nil
	}

	rv := reflect.ValueOf(handler)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	// Find the config file path from the already-parsed flag value.
	configPath := stringFieldForFlag(rv, fields, configFlagName)
	if configPath == "" {
		return nil
	}

	explicit := fs.IsSet(configFlagName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !explicit {
			return nil
		}
		return fmt.Errorf("reading config file %q: %w", configPath, err)
	}

	// Process via loader (hujson + env replacement)
	var raw json.RawMessage
	if err := loader.LoadConfig(data, &raw); err != nil {
		return fmt.Errorf("parsing config file %q: %w", configPath, err)
	}

	rawStr := string(raw)

	for _, fi := range fields {
		if fi.configKey == "" {
			continue
		}

		// Don't override fields explicitly set by CLI flags
		if fi.flagLong != "" && fs.IsSet(fi.flagLong) {
			continue
		}

		var section string
		if fi.configKey == "." {
			section = rawStr
		} else {
			result := gjson.Get(rawStr, fi.configKey)
			if !result.Exists() {
				continue
			}
			section = result.Raw
		}

		if section == "" {
			continue
		}

		fieldVal := fieldByIndex(rv, fi.fieldIndex)
		if err := unmarshalIntoField(fieldVal, section); err != nil {
			return fmt.Errorf("config key %q: %w", fi.configKey, err)
		}
	}

	return nil
}

// stringFieldForFlag returns the current string value of the flag's struct field.
func stringFieldForFlag(rv reflect.Value, fields []fieldInfo, flagName string) string {
	for _, fi := range fields {
		if fi.flagLong != flagName {
			continue
		}
		fieldVal := fieldByIndex(rv, fi.fieldIndex)
		if fieldVal.Kind() == reflect.String {
			return fieldVal.String()
		}
		if fieldVal.Kind() == reflect.Ptr && !fieldVal.IsNil() && fieldVal.Elem().Kind() == reflect.String {
			return fieldVal.Elem().String()
		}
	}
	return ""
}

// unmarshalIntoField unmarshals a JSON string into the given reflect.Value.
func unmarshalIntoField(v reflect.Value, jsonStr string) error {
	if !v.CanAddr() {
		return fmt.Errorf("field is not addressable")
	}
	ptr := v.Addr().Interface()
	return json.Unmarshal([]byte(jsonStr), ptr)
}
