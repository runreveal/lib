package cli

import (
	"encoding"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// flagDef holds the definition of a single flag.
type flagDef struct {
	long     string // long name (without --)
	short    string // short alias (single char, without -)
	usage    string
	defVal   string
	typeName string // for help display
	setValue func(s string) error
	getBool  func() bool // non-nil only for bool flags
}

// FlagSet is a parsed set of flag definitions.
type FlagSet struct {
	defs     []*flagDef
	byLong   map[string]*flagDef
	byShort  map[string]*flagDef
	explicit map[string]bool // flags explicitly set
}

func newFlagSet() *FlagSet {
	return &FlagSet{
		byLong:   make(map[string]*flagDef),
		byShort:  make(map[string]*flagDef),
		explicit: make(map[string]bool),
	}
}

func (fs *FlagSet) add(def *flagDef) error {
	if def.long != "" {
		if _, exists := fs.byLong[def.long]; exists {
			return fmt.Errorf("duplicate flag --%s", def.long)
		}
		fs.byLong[def.long] = def
	}
	if def.short != "" {
		if _, exists := fs.byShort[def.short]; exists {
			return fmt.Errorf("duplicate short flag -%s", def.short)
		}
		fs.byShort[def.short] = def
	}
	fs.defs = append(fs.defs, def)
	return nil
}

// IsSet reports whether the named flag was explicitly set.
func (fs *FlagSet) IsSet(name string) bool {
	return fs.explicit[name]
}

// DumpValues returns a map of flag name → current value string.
// For a FlagSet obtained after parsing, this reflects defaults plus any
// explicit flag values (stored in defVal after setValue is called).
func (fs *FlagSet) DumpValues() map[string]any {
	m := make(map[string]any, len(fs.defs))
	for _, d := range fs.defs {
		if fs.explicit[d.long] {
			m[d.long] = "(set)"
		} else {
			m[d.long] = d.defVal
		}
	}
	return m
}

// Parse parses args, sets flag values, and returns remaining positional args.
func (fs *FlagSet) Parse(args []string) ([]string, error) {
	var positional []string
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			i++
			continue
		}

		var err error
		var advance int
		if strings.HasPrefix(arg, "--") {
			advance, err = fs.parseLong(arg[2:], args[i+1:])
		} else {
			advance, err = fs.parseShort(arg[1:], args[i+1:])
		}
		if err != nil {
			return nil, err
		}
		i += 1 + advance
	}
	return positional, nil
}

func (fs *FlagSet) parseLong(raw string, remaining []string) (int, error) {
	name, val, hasEq := strings.Cut(raw, "=")
	def, ok := fs.byLong[name]
	if !ok {
		return 0, fmt.Errorf("unknown flag: --%s", name)
	}

	if def.getBool != nil {
		// Bool flag: value is optional
		if hasEq {
			if err := def.setValue(val); err != nil {
				return 0, fmt.Errorf("invalid value %q for --%s: %w", val, name, err)
			}
		} else {
			if err := def.setValue("true"); err != nil {
				return 0, err
			}
		}
		fs.explicit[name] = true
		return 0, nil
	}

	if hasEq {
		if err := def.setValue(val); err != nil {
			return 0, fmt.Errorf("invalid value %q for --%s: %w", val, name, err)
		}
		fs.explicit[name] = true
		return 0, nil
	}

	if len(remaining) == 0 {
		return 0, fmt.Errorf("flag --%s requires a value", name)
	}
	if err := def.setValue(remaining[0]); err != nil {
		return 0, fmt.Errorf("invalid value %q for --%s: %w", remaining[0], name, err)
	}
	fs.explicit[name] = true
	return 1, nil
}

func (fs *FlagSet) parseShort(raw string, remaining []string) (int, error) {
	// raw is everything after the leading -
	// e.g. "abc" for -abc, "v" for -v, "f=val" for -f=val

	// Handle -f=value
	if idx := strings.IndexByte(raw, '='); idx == 1 {
		char := string(raw[0])
		val := raw[2:]
		def, ok := fs.byShort[char]
		if !ok {
			return 0, fmt.Errorf("unknown flag: -%s", char)
		}
		if err := def.setValue(val); err != nil {
			return 0, fmt.Errorf("invalid value %q for -%s: %w", val, char, err)
		}
		fs.explicit[def.long] = true
		return 0, nil
	}

	// Single short flag
	if len(raw) == 1 {
		char := raw
		def, ok := fs.byShort[char]
		if !ok {
			return 0, fmt.Errorf("unknown flag: -%s", char)
		}
		if def.getBool != nil {
			if err := def.setValue("true"); err != nil {
				return 0, err
			}
			fs.explicit[def.long] = true
			return 0, nil
		}
		if len(remaining) == 0 {
			return 0, fmt.Errorf("flag -%s requires a value", char)
		}
		if err := def.setValue(remaining[0]); err != nil {
			return 0, fmt.Errorf("invalid value %q for -%s: %w", remaining[0], char, err)
		}
		fs.explicit[def.long] = true
		return 1, nil
	}

	// Multiple short bool flags combined: -abc
	for j, ch := range raw {
		char := string(ch)
		def, ok := fs.byShort[char]
		if !ok {
			return 0, fmt.Errorf("unknown flag: -%s", char)
		}
		if def.getBool == nil {
			// Non-bool flag: rest of the string is the value
			val := raw[j+1:]
			if val == "" {
				if len(remaining) == 0 {
					return 0, fmt.Errorf("flag -%s requires a value", char)
				}
				if err := def.setValue(remaining[0]); err != nil {
					return 0, fmt.Errorf("invalid value %q for -%s: %w", remaining[0], char, err)
				}
				fs.explicit[def.long] = true
				return 1, nil
			}
			if err := def.setValue(val); err != nil {
				return 0, fmt.Errorf("invalid value %q for -%s: %w", val, char, err)
			}
			fs.explicit[def.long] = true
			return 0, nil
		}
		if err := def.setValue("true"); err != nil {
			return 0, err
		}
		fs.explicit[def.long] = true
	}
	return 0, nil
}

// makeFlagDef creates a flagDef for a given field pointer.
// ptr must be a pointer to the field value.
func makeFlagDef(long, short, usage, defVal string, ptr any) (*flagDef, error) {
	def := &flagDef{
		long:  long,
		short: short,
		usage: usage,
		defVal: defVal,
	}

	switch p := ptr.(type) {
	case *string:
		def.typeName = "string"
		def.setValue = func(s string) error { *p = s; return nil }
	case **string:
		def.typeName = "string"
		def.setValue = func(s string) error { *p = &s; return nil }
	case *bool:
		def.typeName = ""
		def.getBool = func() bool { return *p }
		def.setValue = func(s string) error {
			v, err := strconv.ParseBool(s)
			if err != nil {
				return err
			}
			*p = v
			return nil
		}
	case **bool:
		def.typeName = ""
		def.getBool = func() bool { return *p != nil && **p }
		def.setValue = func(s string) error {
			v, err := strconv.ParseBool(s)
			if err != nil {
				return err
			}
			*p = &v
			return nil
		}
	case *int:
		def.typeName = "int"
		def.setValue = func(s string) error {
			v, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return err
			}
			*p = int(v)
			return nil
		}
	case **int:
		def.typeName = "int"
		def.setValue = func(s string) error {
			v, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return err
			}
			iv := int(v)
			*p = &iv
			return nil
		}
	case *int64:
		def.typeName = "int"
		def.setValue = func(s string) error {
			v, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return err
			}
			*p = v
			return nil
		}
	case **int64:
		def.typeName = "int"
		def.setValue = func(s string) error {
			v, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return err
			}
			*p = &v
			return nil
		}
	case *uint:
		def.typeName = "uint"
		def.setValue = func(s string) error {
			v, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return err
			}
			*p = uint(v)
			return nil
		}
	case **uint:
		def.typeName = "uint"
		def.setValue = func(s string) error {
			v, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return err
			}
			uv := uint(v)
			*p = &uv
			return nil
		}
	case *uint64:
		def.typeName = "uint"
		def.setValue = func(s string) error {
			v, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return err
			}
			*p = v
			return nil
		}
	case **uint64:
		def.typeName = "uint"
		def.setValue = func(s string) error {
			v, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return err
			}
			*p = &v
			return nil
		}
	case *float64:
		def.typeName = "float"
		def.setValue = func(s string) error {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*p = v
			return nil
		}
	case **float64:
		def.typeName = "float"
		def.setValue = func(s string) error {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return err
			}
			*p = &v
			return nil
		}
	case *time.Duration:
		def.typeName = "duration"
		def.setValue = func(s string) error {
			v, err := time.ParseDuration(s)
			if err != nil {
				return err
			}
			*p = v
			return nil
		}
	case **time.Duration:
		def.typeName = "duration"
		def.setValue = func(s string) error {
			v, err := time.ParseDuration(s)
			if err != nil {
				return err
			}
			*p = &v
			return nil
		}
	case *[]string:
		def.typeName = "strings"
		def.setValue = func(s string) error {
			*p = append(*p, s)
			return nil
		}
	default:
		// Check for TextUnmarshaler
		if tu, ok := ptr.(encoding.TextUnmarshaler); ok {
			def.typeName = "string"
			def.setValue = func(s string) error {
				return tu.UnmarshalText([]byte(s))
			}
		} else {
			return nil, fmt.Errorf("unsupported flag type %T", ptr)
		}
	}

	return def, nil
}
