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

// AntConfig is a small, zero-dependency configuration helper that applies
// values to a tagged struct from (in order): defaults, config file (JSON/JSONC),
// .env file, OS environment variables, and command-line flags.
//
// Use New() to construct, MustSetConfig/SetConfig to register your struct
// pointer, optionally BindConfigFlags to register flags on a flag.FlagSet,
// then call WriteConfigValues() to apply.
type AntConfig struct {
	envPath    string
	configPath string
	// flagArgs optionally holds CLI args to parse (e.g., os.Args[1:]).
	// When empty, WriteConfigValues will fall back to os.Args[1:].
	flagArgs []string
	// flagPrefix, if set, is prepended to all CLI flags defined via `flag:"name"`.
	// For example, with flagPrefix="config-" and tag `flag:"secret"`, accepted flag
	// forms include: --config-secret=value, --config-secret value, or --config-secret (bool true).
	flagPrefix string
	// flagSet, if provided, will be populated via BindConfigFlags and consulted for parsed values.
	flagSet *flag.FlagSet
	// cfgRef holds the config pointer used for reflection when binding flags.
	cfgRef any
}

// New constructs a new AntConfig with default settings.
func New() *AntConfig {
	return &AntConfig{}
}

// SetFlagArgs sets the CLI arguments that should be used for flag overrides.
// If not provided, WriteConfigValues falls back to os.Args[1:].
func (c *AntConfig) SetFlagArgs(args []string) {
	c.flagArgs = args
}

// SetFlagPrefix sets an optional CLI flag prefix (e.g., "config-").
func (c *AntConfig) SetFlagPrefix(prefix string) {
	c.flagPrefix = prefix
}

// EnvPath returns the configured .env path, if any.
func (a *AntConfig) EnvPath() string { return a.envPath }

// ConfigPath returns the configured config file path, if any.
func (a *AntConfig) ConfigPath() string { return a.configPath }

// FlagPrefix returns the CLI flag prefix, if any.
func (a *AntConfig) FlagPrefix() string { return a.flagPrefix }

// FlagArgs returns a copy of the configured flag args slice.
func (a *AntConfig) FlagArgs() []string {
	if a.flagArgs == nil {
		return nil
	}
	dup := make([]string, len(a.flagArgs))
	copy(dup, a.flagArgs)
	return dup
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

// MustSetConfig is like SetConfig but panics on error. It returns the receiver
// to allow simple chaining: antconfig.New().MustSetConfig(&cfg).
func (a *AntConfig) MustSetConfig(cfg any) *AntConfig {
	if err := a.SetConfig(cfg); err != nil {
		panic(err)
	}
	return a
}

// BindConfigFlags registers flags for all fields tagged with `flag:"name"` onto the provided FlagSet.
// It respects the configured prefix (via SetFlagPrefix) for the CLI names. This method does not parse
// or apply flags; call fs.Parse(...) yourself, then WriteConfigValues to apply. It also binds the
// FlagSet to AntConfig so WriteConfigValues reads values from it. Requires SetConfig to be called first.
func (a *AntConfig) BindConfigFlags(fs *flag.FlagSet) error {
	if a.cfgRef == nil {
		return fmt.Errorf("BindConfigFlags requires SetConfig to be called first")
	}
	// Collect flag fields (and related metadata like optional descriptions)
	fields, err := findFieldsWithTag("flag", a.cfgRef)
	if err != nil {
		return err
	}
	for _, f := range fields {
		name := f.tagvalue
		cli := name
		if a.flagPrefix != "" {
			cli = a.flagPrefix + name
		}
		usage := ""
		if f.tags != nil {
			usage = f.tags["desc"]
		}
		switch f.fieldValue.Kind() {
		case reflect.Bool:
			fs.Bool(cli, false, usage)
		default:
			fs.String(cli, "", usage)
		}
	}
	a.flagSet = fs
	return nil
}

// MustBindConfigFlags is like BindConfigFlags but panics on error. It returns
// the receiver to allow simple chaining with New()/MustSetConfig.
func (a *AntConfig) MustBindConfigFlags(fs *flag.FlagSet) *AntConfig {
	if err := a.BindConfigFlags(fs); err != nil {
		panic(err)
	}
	return a
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
// If a flag prefix is set, the returned CLI names include the prefix.
func (a *AntConfig) ListFlags(c any) ([]FlagSpec, error) {
	flagFields, err := findFieldsWithTag("flag", c)
	if err != nil {
		return nil, err
	}
	out := make([]FlagSpec, 0, len(flagFields))
	for _, f := range flagFields {
		name := f.tagvalue
		cli := name
		if a.flagPrefix != "" {
			cli = a.flagPrefix + name
		}
		out = append(out, FlagSpec{
			Name: name,
			CLI:  cli,
			Kind: strings.ToLower(f.fieldValue.Kind().String()),
		})
	}
	return out, nil
}

// EnvHelpString builds a help section for environment variables that can
// configure fields of the registered config struct. It returns a string
// formatted to append after flag usage output, using the same two-space
// indentation convention as flag.PrintDefaults.
// Requires SetConfig to have been called; otherwise returns an empty string.
func (a *AntConfig) EnvHelpString() string {
	if a.cfgRef == nil {
		return ""
	}
	fields, err := findFieldsWithTag("env", a.cfgRef)
	if err != nil || len(fields) == 0 {
		return ""
	}
	// Build two columns: col1 = ENV (default "...") if default exists; col2 = description
	type row struct{ col1, col2 string }
	rows := make([]row, 0, len(fields))
	max := 0
	for _, f := range fields {
		envName := f.tagvalue
		def := ""
		if f.tags != nil && f.tags["default"] != "" {
			def = fmt.Sprintf(" (default %q)", f.tags["default"])
		}
		col1 := envName + def
		if len(col1) > max {
			max = len(col1)
		}
		desc := ""
		if f.tags != nil {
			desc = f.tags["desc"]
		}
		rows = append(rows, row{col1: col1, col2: desc})
	}
	var b strings.Builder
	b.WriteString("Environment variables:\n")
	for _, r := range rows {
		b.WriteString(r.col1)
		// At least one space between columns; align by max col1 width
		pad := max - len(r.col1) + 1
		if pad < 1 {
			pad = 1
		}
		b.WriteString(strings.Repeat(" ", pad))
		if r.col2 != "" {
			b.WriteString("- " + r.col2)
		}
		b.WriteString("\n")
	}
	return b.String()
}

//

// SetEnvPath sets the path to a .env file and validates it exists. When not set,
// WriteConfigValues will auto-discover a .env in the current working directory.
func (c *AntConfig) SetEnvPath(path string) error {
	c.envPath = path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrEnvFileNotFound, path)
	}
	return nil
}

// SetConfigPath sets the path to a JSON/JSONC config file and validates it exists.
// When not set, WriteConfigValues will auto-discover config.jsonc or config.json
// by walking upward from the current working directory.
func (c *AntConfig) SetConfigPath(path string) error {
	c.configPath = path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrConfigNotFound, path)
	}
	return nil
}

// WriteConfigValues applies configuration values to the struct registered via
// SetConfig/MustSetConfig, in this precedence order:
//  1. default values from `default:"…"` tags
//  2. config file (JSON/JSONC) from SetConfigPath or auto-discovery
//  3. .env file from SetEnvPath or auto-discovery (does not override existing OS env)
//  4. OS environment variables from `env:"NAME"` tags (non-empty values override)
//  5. command-line flags from a bound FlagSet (BindConfigFlags) or from SetFlagArgs/os.Args
//
// Returns an error on invalid inputs, I/O, or parsing failures.
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
	if a.configPath != "" {
		data, err := os.ReadFile(a.configPath)
		if err != nil {
			return fmt.Errorf("error reading config file %s: %w", a.configPath, err)
		}
		js := ToJSON(data)
		if err := json.Unmarshal(js, c); err != nil {
			return fmt.Errorf("error parsing config file %s: %w", a.configPath, err)
		}
	} else {
		// Auto-discover config file from working directory upwards
		// Try common names in order
		candidates := []string{"config.jsonc", "config.json"}
		for _, name := range candidates {
			if path, err := LocateFromWorkingDirUp(name); err == nil && path != "" {
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
	if a.envPath != "" {
		if err := loadDotEnv(a.envPath); err != nil {
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
		var values map[string]*string
		if a.flagSet != nil {
			values = map[string]*string{}
			a.flagSet.Visit(func(f *flag.Flag) {
				v := f.Value.String()
				values[f.Name] = &v
			})
		} else {
			args := a.flagArgs
			if len(args) == 0 && len(os.Args) > 1 {
				args = os.Args[1:]
			}
			values = parseArgsToFlagMap(args, a.flagPrefix)
		}
		if err := assignFlagsFromMap(flagFields, values, a.flagPrefix); err != nil {
			return fmt.Errorf("error processing flags: %v", err)
		}
	}

	return nil
}

// LocateFromExeUp searches for filename starting from the directory of the
// current executable and then walking upward up to 10 levels. Returns the
// first match or ErrConfigNotFound.
func LocateFromExeUp(filename string) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("Error getting executable path: %v\n", err)
		return "", err
	}
	return searchUpwards(filepath.Dir(exePath), filename)
}

// LocateFromWorkingDirUp searches for filename starting from the current working
// directory and then walking upward up to 10 levels. Returns the first match or
// ErrConfigNotFound.
func LocateFromWorkingDirUp(filename string) (string, error) {
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
	fieldValue reflect.Value
	tagvalue   string
	// tags holds commonly used tag values for this field (e.g., "default",
	// "env", "flag", "desc"). The requested tag's value is also
	// accessible via tagvalue for convenience.
	tags map[string]string
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
			tags := map[string]string{
				"default": fieldType.Tag.Get("default"),
				"env":     fieldType.Tag.Get("env"),
				"flag":    fieldType.Tag.Get("flag"),
				"desc":    fieldType.Tag.Get("desc"),
			}
			fields = append(fields, fieldWithTagValue{
				fieldValue: fieldValue,
				tagvalue:   tagValue,
				tags:       tags,
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

		fieldVal := row.fieldValue
		if !fieldVal.CanSet() {
			continue
		}
		parseCtx := fmt.Sprintf("env var '%s' ('%s')", row.tagvalue, envValStr)
		unsupportedCtx := fmt.Sprintf("env var '%s'", row.tagvalue)
		if err := setFieldFromString(fieldVal, envValStr, parseCtx, unsupportedCtx, true); err != nil {
			return err
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
		fieldVal := row.fieldValue
		if !fieldVal.CanSet() {
			continue
		}
		ctx := fmt.Sprintf("default value '%s'", row.tagvalue)
		if err := setFieldFromString(fieldVal, row.tagvalue, ctx, ctx, true); err != nil {
			return err
		}
	}
	return nil
}

// (moved) ListFlags and FlagSpec are defined above the writer for clarity.

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

		fieldVal := row.fieldValue
		if !fieldVal.CanSet() {
			continue
		}

		// For flags, do not ignore unsupported slice types
		parseCtx := fmt.Sprintf("flag --%s=%q", name, val)
		unsupportedCtx := fmt.Sprintf("flag --%s", name)
		if err := setFieldFromString(fieldVal, val, parseCtx, unsupportedCtx, false); err != nil {
			return err
		}
	}
	return nil
}

// parseArgsToFlagMap builds a map of flag name -> value string pointer by parsing
// args. It supports --name=value, --name value, and presence-only booleans.
// If a prefix is configured, de-prefixed keys are also included.
func parseArgsToFlagMap(args []string, prefix string) map[string]*string {
	values := map[string]*string{}
	if len(args) == 0 {
		return values
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) == 0 {
			continue
		}
		if !(len(a) >= 2 && a[0] == '-') {
			continue
		}
		// strip leading dashes
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
		if eq := strings.IndexByte(keyAndMaybe, '='); eq >= 0 {
			key = keyAndMaybe[:eq]
			v := keyAndMaybe[eq+1:]
			valStr = &v
		} else {
			if i+1 < len(args) && !(len(args[i+1]) > 0 && args[i+1][0] == '-') {
				v := args[i+1]
				valStr = &v
				i++
			} else {
				t := "true"
				valStr = &t
			}
		}
		values[key] = valStr
		if prefix != "" && strings.HasPrefix(key, prefix) {
			k := strings.TrimPrefix(key, prefix)
			if k != "" {
				values[k] = valStr
			}
		}
	}
	return values
}

// setFieldFromString converts the provided string to the type of fieldVal and sets it.
// parseCtx is used in parse error messages (e.g., "flag --name=\"val\"").
// unsupportedCtx is used for unsupported type errors (e.g., "flag --name").
// If ignoreNonIntSlice is true, slices whose element type is not int are ignored
// (used for defaults/env). When false, an error is returned (used for flags).
func setFieldFromString(fieldVal reflect.Value, s string, parseCtx, unsupportedCtx string, ignoreNonIntSlice bool) error {
	switch fieldVal.Kind() {
	case reflect.String:
		fieldVal.SetString(s)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		iv, err := strconv.ParseInt(s, 10, fieldVal.Type().Bits())
		if err != nil {
			return fmt.Errorf("could not parse %s to int: %w", parseCtx, err)
		}
		fieldVal.SetInt(iv)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uv, err := strconv.ParseUint(s, 10, fieldVal.Type().Bits())
		if err != nil {
			return fmt.Errorf("could not parse %s to uint: %w", parseCtx, err)
		}
		fieldVal.SetUint(uv)
		return nil
	case reflect.Bool:
		bv, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("could not parse %s to bool: %w", parseCtx, err)
		}
		fieldVal.SetBool(bv)
		return nil
	case reflect.Float32, reflect.Float64:
		fv, err := strconv.ParseFloat(s, fieldVal.Type().Bits())
		if err != nil {
			return fmt.Errorf("could not parse %s to float: %w", parseCtx, err)
		}
		fieldVal.SetFloat(fv)
		return nil
	case reflect.Slice:
		if fieldVal.Type().Elem().Kind() == reflect.Int {
			var intSlice []int
			if err := json.Unmarshal([]byte(s), &intSlice); err != nil {
				return fmt.Errorf("could not parse %s to []int: %w", parseCtx, err)
			}
			fieldVal.Set(reflect.ValueOf(intSlice))
			return nil
		}
		if ignoreNonIntSlice {
			return nil
		}
		return fmt.Errorf("unsupported slice type for %s: %s", unsupportedCtx, fieldVal.Type().String())
	default:
		return fmt.Errorf("unsupported field type for %s: %s", unsupportedCtx, fieldVal.Kind())
	}
}
