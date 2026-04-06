package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"

	"github.com/runreveal/lib/loader"
	sgjson "github.com/segmentio/encoding/json"
	"github.com/tidwall/gjson"
)

// envVarRegex matches JSON string values that are exactly an env-var reference,
// e.g. "$MY_VAR". The whole quoted token is replaced with the env var's value.
var envVarRegex = regexp.MustCompile(`"\$([A-Za-z_][A-Za-z0-9_]*)"`)

// replaceEnvInJSON replaces "$VAR" tokens inside JSON string values with the
// corresponding environment variable's value, properly re-encoded as JSON.
func replaceEnvInJSON(data []byte) []byte {
	return envVarRegex.ReplaceAllFunc(data, func(match []byte) []byte {
		name := string(match[2 : len(match)-1]) // strip leading `"$` and trailing `"`
		val := os.Getenv(name)
		encoded, err := json.Marshal(val)
		if err != nil {
			return match // should never happen for a plain string
		}
		return encoded
	})
}

// resolveConfigJSON finds the config file path (from globals or handler),
// reads and processes it, and returns the JSON string. Returns "" if no
// config file is available.
func resolveConfigJSON(
	handler Runnable,
	globals any,
	fs *FlagSet,
	configFlagName string,
	globalFields []fieldInfo,
	handlerFields []fieldInfo,
) (string, error) {
	if _, ok := fs.byLong[configFlagName]; !ok {
		return "", nil
	}

	// Find the config file path — check globals first, then handler.
	var configPath string
	if globals != nil {
		rv := reflect.ValueOf(globals)
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
		configPath = stringFieldForFlag(rv, globalFields, configFlagName)
	}
	if configPath == "" {
		rv := reflect.ValueOf(handler)
		if rv.Kind() == reflect.Ptr {
			rv = rv.Elem()
		}
		configPath = stringFieldForFlag(rv, handlerFields, configFlagName)
	}
	if configPath == "" {
		return "", nil
	}

	explicit := fs.IsSet(configFlagName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !explicit {
			return "", nil
		}
		return "", fmt.Errorf("reading config file %q: %w", configPath, err)
	}

	// Process via loader (hujson standardisation + env var replacement).
	var raw json.RawMessage
	if err := loader.LoadConfig(data, &raw); err != nil {
		return "", fmt.Errorf("parsing config file %q: %w", configPath, err)
	}

	return string(replaceEnvInJSON(raw)), nil
}

// applyConfigTags applies config:"key" struct tags on the handler.
func applyConfigTags(handler Runnable, fs *FlagSet, fields []fieldInfo, rawJSON string) error {
	rv := reflect.ValueOf(handler)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	for _, fi := range fields {
		if fi.configKey == "" {
			continue
		}
		// Don't override fields explicitly set by CLI flags
		if fi.flagLong != "" && fs.IsSet(fi.flagLong) {
			continue
		}

		section := extractSection(rawJSON, fi.configKey)
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

// applyConfigBindings applies ConfigAt bindings.
func applyConfigBindings(bindings []configBinding, rawJSON string) error {
	for _, b := range bindings {
		section := extractSection(rawJSON, b.key)
		if section == "" {
			continue
		}
		if err := sgjson.Unmarshal([]byte(section), b.dst); err != nil {
			return fmt.Errorf("config key %q: %w", b.key, err)
		}
	}
	return nil
}

// extractSection extracts a JSON section by key path.
func extractSection(rawJSON, key string) string {
	if key == "." {
		return rawJSON
	}
	result := gjson.Get(rawJSON, key)
	if !result.Exists() {
		return ""
	}
	return result.Raw
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
	return sgjson.Unmarshal([]byte(jsonStr), ptr)
}
