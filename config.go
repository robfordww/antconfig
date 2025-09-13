package antconfig

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// Errors
var ErrConfigNotFound = errors.New("config file not found")
var ErrEnvFileNotFound = errors.New("environment file not found")

type AntConfig struct {
    EnvPath    string
    ConfigPath string
    // FlagArgs optionally holds CLI args to parse (e.g., os.Args[1:]).
    // When empty, WriteConfigValues will fall back to os.Args[1:].
	FlagArgs []string
	// FlagPrefix, if set, is prepended to all CLI flags defined via `flag:"name"`.
	// For example, with FlagPrefix="config-" and tag `flag:"secret"`, accepted flag
	// forms include: --config-secret=value, --config-secret value, or --config-secret (bool true).
    FlagPrefix string
    // FlagSet, if provided, will be populated via BindConfigFlags and consulted for parsed values.
    FlagSet *flag.FlagSet
    // cfgRef holds the config pointer used for reflection when binding flags.
    cfgRef any
}

// SetFlagArgs sets the CLI arguments that should be used for flag overrides.
func (c *AntConfig) SetFlagArgs(args []string) {
	c.FlagArgs = args
}

// SetFlagPrefix sets an optional CLI flag prefix (e.g., "config-").
func (c *AntConfig) SetFlagPrefix(prefix string) {
    c.FlagPrefix = prefix
}

// SetConfig stores a reference to the config pointer for later operations
// like BindConfigFlags. cfg must be a non-nil pointer to a struct.
func (a *AntConfig) SetConfig(cfg any) error {
    if cfg == nil {
        return fmt.Errorf("expected a non-nil pointer to a struct, got <nil>")
    }
    v := reflect.ValueOf(cfg)
    if v.Kind() != reflect.Ptr || v.IsNil() || v.Elem().Kind() != reflect.Struct {
        return fmt.Errorf("expected a non-nil pointer to a struct, got %s", v.Kind())
    }
    a.cfgRef = cfg
    return nil
}

// BindConfigFlags registers flags for all fields tagged with `flag:"name"` onto the provided FlagSet.
// It respects AntConfig.FlagPrefix for the CLI names. This method does not parse or apply flags;
// call fs.Parse(...) yourself, then WriteConfigValues to apply. It also binds the FlagSet to AntConfig
// so WriteConfigValues reads values from it. Requires SetConfig to have been called.
func (a *AntConfig) BindConfigFlags(fs *flag.FlagSet) error {
    if a.cfgRef == nil {
        return fmt.Errorf("BindConfigFlags requires SetConfig to be called first")
    }
    fields, err := findFieldsWithTag("flag", a.cfgRef)
    if err != nil {
        return err
    }
	for _, f := range fields {
		name := f.tagvalue
		cli := name
		if a.FlagPrefix != "" {
			cli = a.FlagPrefix + name
		}
		usage := "AntConfig override"
		switch f.pvalue.Kind() {
		case reflect.Bool:
			fs.Bool(cli, false, usage)
		default:
			fs.String(cli, "", usage)
		}
	}
    a.FlagSet = fs
    return nil
}

func (c *AntConfig) SetEnvPath(path string) error {
	// check if file exists
	c.EnvPath = path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrEnvFileNotFound, path)
	}
	return nil
}

func (c *AntConfig) SetConfigPath(path string) error {
	// check if file exists
	c.ConfigPath = path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrConfigNotFound, path)
	}
	return nil
}

func (a *AntConfig) WriteConfigValues() error {
    if a.cfgRef == nil {
        return fmt.Errorf("WriteConfigValues requires SetConfig to be called first")
    }
    c := a.cfgRef
    // Make sure c is a pointer to a struct
    if reflect.TypeOf(c).Kind() != reflect.Ptr || reflect.TypeOf(c).Elem().Kind() != reflect.Struct {
        return fmt.Errorf("expected a pointer to a struct, got %s", reflect.TypeOf(c).Kind())
    }

	// Set default values based on struct tags
	fields, err := findFieldsWithTag("default", c)
	if err != nil {
		return fmt.Errorf("error finding fields with 'default' tag: %v", err)
	}
	if err := setDefaultValues(fields); err != nil {
		return fmt.Errorf("error setting default values: %v", err)
	}

    // Merge configuration file (JSON/JSONC) over defaults, if provided
	if a.ConfigPath != "" {
		data, err := os.ReadFile(a.ConfigPath)
		if err != nil {
			return fmt.Errorf("error reading config file %s: %w", a.ConfigPath, err)
		}
		js := ToJSON(data)
		if err := json.Unmarshal(js, c); err != nil {
			return fmt.Errorf("error parsing config file %s: %w", a.ConfigPath, err)
		}
	} else {
		// Auto-discover config file from working directory upwards
		// Try common names in order
		candidates := []string{"config.jsonc", "config.json"}
		for _, name := range candidates {
			if path, err := LocateFromWorkingDir(name); err == nil && path != "" {
				if data, rerr := os.ReadFile(path); rerr == nil {
					js := ToJSON(data)
					if uerr := json.Unmarshal(js, c); uerr != nil {
						return fmt.Errorf("error parsing discovered config %s: %w", path, uerr)
					}
				}
				break
			}
		}
	}

	// Process environment variables based on .env file

	// Load .env file into process environment if configured, otherwise auto-discover in CWD.
	// .env is lower priority than explicit env variables.
	if a.EnvPath != "" {
		if err := loadDotEnv(a.EnvPath); err != nil {
			return fmt.Errorf("error loading .env file: %w", err)
		}
	} else {
		if wd, err := os.Getwd(); err == nil {
			candidate := filepath.Join(wd, ".env")
			if _, statErr := os.Stat(candidate); statErr == nil {
				if err := loadDotEnv(candidate); err != nil {
					return fmt.Errorf("error loading discovered .env file: %w", err)
				}
			}
		}
	}

	// Process environment variables based on system environment
	fields, err = findFieldsWithTag("env", c)
	if err != nil {
		return fmt.Errorf("error finding fields with 'env' tag: %v", err)
	}
	if len(fields) > 0 {
		if err := processEnvironment(fields); err != nil {
			return fmt.Errorf("error processing environment variables: %v", err)
		}
	}

	// Process command-line flag overrides (highest precedence)
	flagFields, err := findFieldsWithTag("flag", c)
	if err != nil {
		return fmt.Errorf("error finding fields with 'flag' tag: %v", err)
	}
	if len(flagFields) > 0 {
		if a.FlagSet != nil {
			values := map[string]*string{}
			a.FlagSet.Visit(func(f *flag.Flag) {
				v := f.Value.String()
				values[f.Name] = &v
			})
			if err := assignFlagsFromMap(flagFields, values, a.FlagPrefix); err != nil {
				return fmt.Errorf("error processing flags from FlagSet: %v", err)
			}
		} else {
			args := a.FlagArgs
			if len(args) == 0 && len(os.Args) > 1 {
				args = os.Args[1:]
			}
			if err := processFlags(flagFields, args, a.FlagPrefix); err != nil {
				return fmt.Errorf("error processing flags: %v", err)
			}
		}
	}

	return nil
}

func LocateFromExe(filename string) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("Error getting executable path: %v\n", err)
		return "", err
	}
	return searchUpwards(filepath.Dir(exePath), filename)
}

func LocateFromWorkingDir(filename string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting working directory: %v\n", err)
		return "", err
	}
	return searchUpwards(wd, filename)
}

func searchUpwards(path, configFile string) (string, error) {
	maxLevels := 10
	for i := 0; i < maxLevels; i++ {
		if _, err := os.Stat(filepath.Join(path, configFile)); err == nil {
			return filepath.Join(path, configFile), nil
		}
		if path == "/" || path == "." {
			return "", fmt.Errorf("%w: %s", ErrConfigNotFound, configFile)
		}
		path = filepath.Dir(path)
		if path == "" {
			return "", fmt.Errorf("%w: %s", ErrConfigNotFound, configFile)
		}
	}
	return "", fmt.Errorf("%w: %s", ErrConfigNotFound, configFile)
}

type fieldWithTagValue struct {
	pvalue   reflect.Value
	tagvalue string
}

// findFieldsWithTag returns a slice of fieldWithTagValue containing settable
// reflect.Value instances for fields with the specified tag. It correctly
// traverses nested structs, including those that are nil pointers.
func findFieldsWithTag(tagname string, s any) ([]fieldWithTagValue, error) {
	var fields []fieldWithTagValue
	v := reflect.ValueOf(s)

	// If s is not a pointer to a struct, it's an error because we can't set fields.
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil, fmt.Errorf("expected a non-nil pointer to a struct, got %s", v.Kind())
	}

	// Get the struct value that the pointer points to.
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected a pointer to a struct, but it points to %s", v.Kind())
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fieldValue := v.Field(i)
		fieldType := t.Field(i)

		// We can only process settable (i.e., exported) fields.
		if !fieldValue.CanSet() {
			continue
		}

		// --- Recursion Logic ---
		// Recurse into nested structs (passed by value).
		// We pass the address to ensure fields within it remain settable.
		if fieldValue.Kind() == reflect.Struct && fieldValue.CanAddr() {
			nestedFields, err := findFieldsWithTag(tagname, fieldValue.Addr().Interface())
			if err != nil {
				return nil, err
			}
			fields = append(fields, nestedFields...)
		}

		// Recurse into nested pointers to structs.
		if fieldValue.Kind() == reflect.Ptr && fieldValue.Type().Elem().Kind() == reflect.Struct {
			// If the pointer is nil, create a new struct instance for it.
			if fieldValue.IsNil() {
				fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
			}
			nestedFields, err := findFieldsWithTag(tagname, fieldValue.Interface())
			if err != nil {
				return nil, err
			}
			fields = append(fields, nestedFields...)
		}

		// --- Tag Processing ---
		// After recursion, process the tag on the current field.
		if tagValue := fieldType.Tag.Get(tagname); tagValue != "" {
			fields = append(fields, fieldWithTagValue{
				pvalue:   fieldValue,
				tagvalue: tagValue,
			})
		}
	}

	return fields, nil
}

// processEnvironment retrieves the environment variable using the tag value, converts
// it to the correct type, and sets the struct field.
func processEnvironment(fieldList []fieldWithTagValue) error {
	for _, row := range fieldList {
		envValStr := os.Getenv(row.tagvalue)
		if envValStr == "" {
			continue
		}

		fieldVal := row.pvalue
		// This check is mostly for safety; findFieldsWithTag should only return settable values.
		if !fieldVal.CanSet() {
			continue
		}

		// Convert the environment variable string to the field's actual type.
		switch fieldVal.Kind() {
		case reflect.String:
			fieldVal.SetString(envValStr)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			intVal, err := strconv.ParseInt(envValStr, 10, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse env var '%s' ('%s') to int: %w", row.tagvalue, envValStr, err)
			}
			fieldVal.SetInt(intVal)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			uintVal, err := strconv.ParseUint(envValStr, 10, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse env var '%s' ('%s') to uint: %w", row.tagvalue, envValStr, err)
			}
			fieldVal.SetUint(uintVal)
		case reflect.Bool:
			boolVal, err := strconv.ParseBool(envValStr)
			if err != nil {
				return fmt.Errorf("could not parse env var '%s' ('%s') to bool: %w", row.tagvalue, envValStr, err)
			}
			fieldVal.SetBool(boolVal)
		case reflect.Float32, reflect.Float64:
			floatVal, err := strconv.ParseFloat(envValStr, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse env var '%s' ('%s') to float: %w", row.tagvalue, envValStr, err)
			}
			fieldVal.SetFloat(floatVal)
		case reflect.Slice:
			if fieldVal.Type().Elem().Kind() == reflect.Int {
				// Parse the slice from the environment variable
				intSlice := []int{}
				if err := json.Unmarshal([]byte(envValStr), &intSlice); err != nil {
					return fmt.Errorf("could not parse env var '%s' ('%s') to []int: %w", row.tagvalue, envValStr, err)
				}
				fieldVal.Set(reflect.ValueOf(intSlice))
			}

		default:
			return fmt.Errorf("unsupported field type for env var '%s': %s", row.tagvalue, fieldVal.Kind())
		}
	}
	return nil
}

// process defaultValues sets default values for fields that have a 'default' tag.
func setDefaultValues(fieldList []fieldWithTagValue) error {
	for _, row := range fieldList {
		if row.tagvalue == "" {
			continue
		}

		fieldVal := row.pvalue
		if !fieldVal.CanSet() {
			continue
		}

		switch fieldVal.Kind() {
		case reflect.String:
			fieldVal.SetString(row.tagvalue)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			intVal, err := strconv.ParseInt(row.tagvalue, 10, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse default value '%s' to int: %w", row.tagvalue, err)
			}
			fieldVal.SetInt(intVal)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			uintVal, err := strconv.ParseUint(row.tagvalue, 10, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse default value '%s' to uint: %w", row.tagvalue, err)
			}
			fieldVal.SetUint(uintVal)
		case reflect.Bool:
			boolVal, err := strconv.ParseBool(row.tagvalue)
			if err != nil {
				return fmt.Errorf("could not parse default value '%s' to bool: %w", row.tagvalue, err)
			}
			fieldVal.SetBool(boolVal)
		case reflect.Float32, reflect.Float64:
			floatVal, err := strconv.ParseFloat(row.tagvalue, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse default value '%s' to float: %w", row.tagvalue, err)
			}
			fieldVal.SetFloat(floatVal)
		case reflect.Slice:
			if fieldVal.Type().Elem().Kind() == reflect.Int {
				intSlice := []int{}
				if err := json.Unmarshal([]byte(row.tagvalue), &intSlice); err != nil {
					return fmt.Errorf("could not parse default value '%s' to []int: %w", row.tagvalue, err)
				}
				fieldVal.Set(reflect.ValueOf(intSlice))
			}
		default:
			return fmt.Errorf("unsupported field type for default value '%s': %s", row.tagvalue, fieldVal.Kind())
		}
	}
	return nil
}

// FlagSpec describes a single CLI flag derived from a struct field with a `flag:"name"` tag.
type FlagSpec struct {
	// Name is the logical flag name from the tag (without prefix), e.g., "secret".
	Name string
	// CLI is the concrete CLI flag including any configured prefix, e.g., "config-secret".
	CLI string
	// Kind is the Go kind for the target field (string, int, bool, float64, slice).
	Kind string
}

// ListFlags returns the set of CLI flags for fields tagged with `flag:"name"`.
// If a.FlagPrefix is set, the returned CLI names include the prefix.
func (a *AntConfig) ListFlags(c any) ([]FlagSpec, error) {
	flagFields, err := findFieldsWithTag("flag", c)
	if err != nil {
		return nil, err
	}
	out := make([]FlagSpec, 0, len(flagFields))
	for _, f := range flagFields {
		name := f.tagvalue
		cli := name
		if a.FlagPrefix != "" {
			cli = a.FlagPrefix + name
		}
		out = append(out, FlagSpec{
			Name: name,
			CLI:  cli,
			Kind: strings.ToLower(f.pvalue.Kind().String()),
		})
	}
	return out, nil
}

// processFlags parses provided args for flags matching the `flag:"name"` tag
// and assigns values with highest precedence. Supported formats:
// --name=value, --name value, and for bools: --name (implies true) or --name=false
func processFlags(fieldList []fieldWithTagValue, args []string, prefix string) error {
	if len(args) == 0 {
		return nil
	}
	// Build a map of flag -> value as string
	values := map[string]*string{}
	// iterate with index to support --name value
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) == 0 {
			continue
		}
		// accept both "--" and "-" prefixes
		if !(len(a) >= 2 && a[0] == '-') {
			continue
		}
		// normalize: strip leading dashes
		j := 0
		for j < len(a) && a[j] == '-' {
			j++
		}
		keyAndMaybe := a[j:]
		if keyAndMaybe == "" {
			continue
		}
		key := keyAndMaybe
		var valStr *string
		// handle --name=value
		if eq := strings.IndexByte(keyAndMaybe, '='); eq >= 0 {
			key = keyAndMaybe[:eq]
			v := keyAndMaybe[eq+1:]
			valStr = &v
		} else {
			// handle --name value
			if i+1 < len(args) && !(len(args[i+1]) > 0 && args[i+1][0] == '-') {
				v := args[i+1]
				valStr = &v
				i++
			} else {
				// presence-only (bool true)
				t := "true"
				valStr = &t
			}
		}
		// last occurrence wins
		values[key] = valStr
		// If a prefix is configured and present, also map the de-prefixed key.
		if prefix != "" && strings.HasPrefix(key, prefix) {
			k := strings.TrimPrefix(key, prefix)
			if k != "" {
				values[k] = valStr
			}
		}
	}

	for _, row := range fieldList {
		name := row.tagvalue
		// Prefer exact match by logical name; if not found, check prefixed form
		valPtr, ok := values[name]
		if !ok || valPtr == nil {
			if prefix != "" {
				if v2, ok2 := values[prefix+name]; ok2 && v2 != nil {
					valPtr = v2
				} else {
					continue
				}
			} else {
				continue
			}
		}
		val := *valPtr

		fieldVal := row.pvalue
		if !fieldVal.CanSet() {
			continue
		}

		switch fieldVal.Kind() {
		case reflect.String:
			fieldVal.SetString(val)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			iv, err := strconv.ParseInt(val, 10, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse flag --%s=%q to int: %w", name, val, err)
			}
			fieldVal.SetInt(iv)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			uv, err := strconv.ParseUint(val, 10, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse flag --%s=%q to uint: %w", name, val, err)
			}
			fieldVal.SetUint(uv)
		case reflect.Bool:
			bv, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("could not parse flag --%s=%q to bool: %w", name, val, err)
			}
			fieldVal.SetBool(bv)
		case reflect.Float32, reflect.Float64:
			fv, err := strconv.ParseFloat(val, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse flag --%s=%q to float: %w", name, val, err)
			}
			fieldVal.SetFloat(fv)
		case reflect.Slice:
			if fieldVal.Type().Elem().Kind() == reflect.Int {
				intSlice := []int{}
				if err := json.Unmarshal([]byte(val), &intSlice); err != nil {
					return fmt.Errorf("could not parse flag --%s=%q to []int: %w", name, val, err)
				}
				fieldVal.Set(reflect.ValueOf(intSlice))
			} else {
				return fmt.Errorf("unsupported slice type for flag --%s: %s", name, fieldVal.Type().String())
			}
		default:
			return fmt.Errorf("unsupported field type for flag --%s: %s", name, fieldVal.Kind())
		}
	}
	return nil
}

// loadDotEnv parses a .env-like file and sets process environment variables
// for keys that are not already explicitly present in the environment.
// This ensures precedence: defaults < .env < OS env < flags.
func loadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		// Only return error if the path was set but unreadable; caller controls existence.
		return err
	}
	lines := strings.Split(string(data), "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Optional "export " prefix
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		// Split at first '='
		eq := strings.IndexByte(line, '=')
		if eq <= 0 { // no '=', or empty key
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if key == "" {
			continue
		}
		// Handle quoted values; for double quotes, unescape common sequences
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			quote := val[0]
			inner := val[1 : len(val)-1]
			if quote == '"' {
				inner = unescapeDoubleQuoted(inner)
			}
			val = inner
		} else {
			// For unquoted values, strip trailing inline comment if preceded by whitespace
			if hash := strings.IndexByte(val, '#'); hash >= 0 {
				// Keep portion before '#', but only if there's whitespace before '#'
				trimmed := strings.TrimRightFunc(val[:hash], func(r rune) bool { return r == ' ' || r == '\t' })
				if len(trimmed) < len(val[:hash]) {
					val = strings.TrimSpace(trimmed)
				}
			}
		}
		if _, exists := os.LookupEnv(key); exists {
			// Do not override explicit env
			continue
		}
		_ = os.Setenv(key, val)
	}
	return nil
}

// unescapeDoubleQuoted handles a minimal set of escape sequences within a double-quoted .env value.
func unescapeDoubleQuoted(s string) string {
	// Replace common escapes: \\ \n \r \t \" and \$
	// We avoid strconv.Unquote to keep control and avoid treating backticks, etc.
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
				continue
			case 'r':
				b.WriteByte('\r')
				continue
			case 't':
				b.WriteByte('\t')
				continue
			case '"':
				b.WriteByte('"')
				continue
			case '\\':
				b.WriteByte('\\')
				continue
			case '$':
				b.WriteByte('$')
				continue
			default:
				// Unknown escape, keep literally the escaped char
				b.WriteByte(s[i])
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// assignFlagsFromMap applies parsed flag values to the struct fields.
func assignFlagsFromMap(fieldList []fieldWithTagValue, values map[string]*string, prefix string) error {
	for _, row := range fieldList {
		name := row.tagvalue
		// Prefer exact match by logical name; if not found, check prefixed form
		valPtr, ok := values[name]
		if !ok || valPtr == nil {
			if prefix != "" {
				if v2, ok2 := values[prefix+name]; ok2 && v2 != nil {
					valPtr = v2
				} else {
					continue
				}
			} else {
				continue
			}
		}
		val := *valPtr

		fieldVal := row.pvalue
		if !fieldVal.CanSet() {
			continue
		}

		switch fieldVal.Kind() {
		case reflect.String:
			fieldVal.SetString(val)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			iv, err := strconv.ParseInt(val, 10, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse flag --%s=%q to int: %w", name, val, err)
			}
			fieldVal.SetInt(iv)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			uv, err := strconv.ParseUint(val, 10, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse flag --%s=%q to uint: %w", name, val, err)
			}
			fieldVal.SetUint(uv)
		case reflect.Bool:
			bv, err := strconv.ParseBool(val)
			if err != nil {
				return fmt.Errorf("could not parse flag --%s=%q to bool: %w", name, val, err)
			}
			fieldVal.SetBool(bv)
		case reflect.Float32, reflect.Float64:
			fv, err := strconv.ParseFloat(val, fieldVal.Type().Bits())
			if err != nil {
				return fmt.Errorf("could not parse flag --%s=%q to float: %w", name, val, err)
			}
			fieldVal.SetFloat(fv)
		case reflect.Slice:
			if fieldVal.Type().Elem().Kind() == reflect.Int {
				intSlice := []int{}
				if err := json.Unmarshal([]byte(val), &intSlice); err != nil {
					return fmt.Errorf("could not parse flag --%s=%q to []int: %w", name, val, err)
				}
				fieldVal.Set(reflect.ValueOf(intSlice))
			} else {
				return fmt.Errorf("unsupported slice type for flag --%s: %s", name, fieldVal.Type().String())
			}
		default:
			return fmt.Errorf("unsupported field type for flag --%s: %s", name, fieldVal.Kind())
		}
	}
	return nil
}

// indexByte returns the index of the first byte b in s, or -1 if not present.
// (removed) indexByte replaced by strings.IndexByte
