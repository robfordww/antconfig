package antconfig

import (
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type TestConfig struct {
	Heading   string `env:"Heading" default:"south"`
	Speed     int    `default:"42"`
	SecretKey string `env:"SecretKey" flag:"secretkey"`
	Database  struct {
		Host    string `env:"DB_HOST" default:"localhost"`
		Ports   []int  `env:"DB_PORT" default:"[5432,3306]"`
		Encrypt bool   `env:"DB_ENCRYPT" flag:"encrypt"`
		Auth    struct {
			User     string `env:"DB_USER" default:"user" flag:"authuser"`
			Password string `env:"DB_PASSWORD" default:"password" flag:"authpassword"`
		}
	}
}

func TestLocateFromExe(t *testing.T) {
	configFile := "configgod.testx"
	path, err := LocateFromExe(configFile)
	if err == nil {
		t.Fatalf("LocateFromExe should have failed, but got: %v", path)
	}
	if path != "" {
		t.Fatalf("LocateFromExe should not have found a path, but got: %s", path)
	}
	// test if error is ErrConfigNotFound
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("Expected error to be ErrConfigNotFound, but got: %v", err)
	}

	// Create the config file in the executable directory for testing; and try again
	// We have to do this because test executables are generated in a temporary directory
	exePath, err := os.Executable()
	if err != nil {
		t.Fatalf("Error getting executable path: %v", err)
	}
	testFilePath := filepath.Join(filepath.Dir(exePath), configFile)
	// make sure the test file does not exist before creating it
	if _, err := os.Stat(testFilePath); !os.IsNotExist(err) {
		t.Fatalf("Test config file already exists: %s", testFilePath)
	}
	if err := os.WriteFile(testFilePath, []byte("test content"), 0644); err != nil {
		t.Fatalf("Error creating test config file: %v", err)
	}
	path, err = LocateFromExe(configFile)
	if err != nil {
		t.Fatalf("LocateFromExe failed after creating test file: %v", err)
	}
	if path == "" {
		t.Fatal("LocateFromExe returned empty path after creating test file")
	}
	t.Logf("Config file found at: %s", path)
	// Clean up the test file
	if err := os.Remove(testFilePath); err != nil {
		t.Fatalf("Error removing test config file: %v", err)
	}
	if !reflect.DeepEqual(path, testFilePath) {
		t.Fatalf("Expected path %s, but got %s", testFilePath, path)
	}
	t.Logf("LocateFromExe test passed, found config file at: %s", path)
}

func TestLocateFromWorkingDir(t *testing.T) {
	// Print working directory for debugging
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting working directory: %v", err)
	}
	t.Logf("Current working directory: %s", wd)
	configFile := "config_test.jsonc"
	path, err := LocateFromWorkingDir(configFile)
	if err != nil {
		t.Fatalf("LocagetFromWorkingDir failed: %v", err)
	}
	if path == "" {
		t.Fatal("LocagetFromWorkingDir returned empty path")
	}
	t.Logf("Config file found at: %s", path)
}

func TestFindFieldsWithTag(t *testing.T) {
	config := TestConfig{}
	fields, err := findFieldsWithTag("env", &config)
	if err != nil {
		t.Fatalf("Error finding fields with tag: %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("Expected to find fields with 'env' tag, but got none")
	}

	t.Logf("Found fields with 'env' tag: %v", fields)
}

func TestFindFieldsWithTag_NonPointerError(t *testing.T) {
	config := TestConfig{}
	if _, err := findFieldsWithTag("env", config); err == nil {
		t.Fatal("expected error when passing non-pointer to findFieldsWithTag")
	}
}

func TestStandardUsage(t *testing.T) {
	config := TestConfig{}
	// Set environment variables for testing
	t.Setenv("Heading", "north")
	t.Setenv("DB_HOST", "testhost")
	t.Setenv("DB_PORT", "[1234,5678]")
	t.Setenv("DB_USER", "testuser")
	t.Setenv("DB_PASSWORD", "testpassword")

    // Set effective config
    antConfig := &AntConfig{}
    if err := antConfig.SetConfig(&config); err != nil { t.Fatal(err) }
    err := antConfig.WriteConfigValues()
	if err != nil {
		t.Fatalf("SetEffectiveConfig failed: %v", err)
	}

	// Check if the values are set correctly
	if config.Heading != "north" {
		t.Errorf("Expected Heading to be 'north', but got '%s'", config.Heading)
	}

	if config.Database.Host != "testhost" {
		t.Errorf("Expected DB_HOST to be 'testhost', but got '%s'", config.Database.Host)
	}
	if len(config.Database.Ports) != 2 || config.Database.Ports[0] != 1234 || config.Database.Ports[1] != 5678 {
		t.Errorf("Expected DB_PORTS to be [1234, 5678], but got %v", config.Database.Ports)
	}
	if config.Database.Auth.User != "testuser" {
		t.Errorf("Expected DB_USER to be 'testuser', but got '%s'", config.Database.Auth.User)
	}
	if config.Database.Auth.Password != "testpassword" {
		t.Errorf("Expected DB_PASSWORD to be 'testpassword', but got '%s'", config.Database.Auth.Password)
	}

}

func TestDefaultValues(t *testing.T) {
	config := TestConfig{}
    antConfig := &AntConfig{}

    // Set effective config without environment variables
    if err := antConfig.SetConfig(&config); err != nil { t.Fatal(err) }
    err := antConfig.WriteConfigValues()
	if err != nil {
		t.Fatalf("SetEffectiveConfig failed: %v", err)
	}

	// Check if default values are set correctly
	if config.Heading != "south" {
		t.Errorf("Expected Heading to be 'south', but got '%s'", config.Heading)
	}
	if config.Speed != 42 {
		t.Errorf("Expected Speed to be 42, but got %d", config.Speed)
	}
	if config.SecretKey != "" {
		t.Errorf("Expected SecretKey to be empty, but got '%s'", config.SecretKey)
	}
	if config.Database.Host != "localhost" {
		t.Errorf("Expected DB_HOST to be 'localhost', but got '%s'", config.Database.Host)
	}
	if len(config.Database.Ports) != 2 || config.Database.Ports[0] != 5432 || config.Database.Ports[1] != 3306 {
		t.Errorf("Expected DB_PORTS to be [5432, 3306], but got %v", config.Database.Ports)
	}
	if config.Database.Encrypt {
		t.Error("Expected DB_ENCRYPT to be false by default, but it is true")
	}

}

func TestFlagOverridesHighestPriority(t *testing.T) {
	cfg := TestConfig{}

	// Set environment values that would be overridden by flags
	t.Setenv("SecretKey", "env-secret")
	t.Setenv("DB_ENCRYPT", "true")
	t.Setenv("DB_USER", "envuser")
	t.Setenv("DB_PASSWORD", "envpass")

	ant := &AntConfig{}
	ant.SetFlagArgs([]string{
		"--secretkey", "cli-secret",
		"--encrypt=false",
		"--authuser=cliuser",
		"--authpassword", "clipass",
	})

    if err := ant.SetConfig(&cfg); err != nil { t.Fatal(err) }
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatalf("SetValues with flags failed: %v", err)
    }

	if cfg.SecretKey != "cli-secret" {
		t.Fatalf("expected SecretKey from flags, got %q", cfg.SecretKey)
	}
	if cfg.Database.Encrypt != false {
		t.Fatalf("expected Encrypt=false from flags, got %v", cfg.Database.Encrypt)
	}
	if cfg.Database.Auth.User != "cliuser" {
		t.Fatalf("expected Auth.User from flags, got %q", cfg.Database.Auth.User)
	}
	if cfg.Database.Auth.Password != "clipass" {
		t.Fatalf("expected Auth.Password from flags, got %q", cfg.Database.Auth.Password)
	}
}

func TestSetEnvPath(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		c := &AntConfig{}
		dir := t.TempDir()
		p := filepath.Join(dir, "missing.env")
		err := c.SetEnvPath(p)
		if !errors.Is(err, ErrEnvFileNotFound) {
			t.Fatalf("expected ErrEnvFileNotFound, got %v", err)
		}
		if c.EnvPath != p {
			t.Fatalf("EnvPath should be set to provided path, got %q", c.EnvPath)
		}
	})
	t.Run("success", func(t *testing.T) {
		c := &AntConfig{}
		dir := t.TempDir()
		p := filepath.Join(dir, ".env")
		if err := os.WriteFile(p, []byte("KEY=VALUE\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := c.SetEnvPath(p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.EnvPath != p {
			t.Fatalf("EnvPath mismatch: %q", c.EnvPath)
		}
	})
}

func TestSetConfigPath(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		c := &AntConfig{}
		dir := t.TempDir()
		p := filepath.Join(dir, "config.jsonc")
		err := c.SetConfigPath(p)
		if !errors.Is(err, ErrConfigNotFound) {
			t.Fatalf("expected ErrConfigNotFound, got %v", err)
		}
		if c.ConfigPath != p {
			t.Fatalf("ConfigPath should be set to provided path, got %q", c.ConfigPath)
		}
	})
	t.Run("success", func(t *testing.T) {
		c := &AntConfig{}
		dir := t.TempDir()
		p := filepath.Join(dir, "config.jsonc")
		if err := os.WriteFile(p, []byte("{}\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := c.SetConfigPath(p); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.ConfigPath != p {
			t.Fatalf("ConfigPath mismatch: %q", c.ConfigPath)
		}
	})
}

func TestLocateFromWorkingDir_NotFound(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	_, err := LocateFromWorkingDir("definitely-not-existing-xyz.jsonc")
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("expected ErrConfigNotFound, got %v", err)
	}
}

func TestSetEffectiveConfig_InvalidInputs(t *testing.T) {
    ant := &AntConfig{}

    // non-pointer
    if err := ant.SetConfig(TestConfig{}); err == nil {
        t.Fatal("expected error for non-pointer input")
    }

    // nil pointer
    var nilCfg *TestConfig
    if err := ant.SetConfig(nilCfg); err == nil {
        t.Fatal("expected error for nil pointer input")
    }

    // pointer to non-struct
    var x int
    if err := ant.SetConfig(&x); err == nil {
        t.Fatal("expected error for pointer to non-struct")
    }
}

func TestTypes_DefaultsAndEnvOverrides(t *testing.T) {
	type TypesCfg struct {
		S string  `env:"S" default:"s0"`
		I int     `env:"I" default:"1"`
		U uint    `env:"U" default:"2"`
		B bool    `env:"B" default:"true"`
		F float64 `env:"F" default:"3.14"`
		L []int   `env:"L" default:"[1,2,3]"`
	}

	ant := &AntConfig{}

	// Defaults only
    var a TypesCfg
    if err := ant.SetConfig(&a); err != nil { t.Fatal(err) }
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatalf("SetEffectiveConfig: %v", err)
    }
	if a.S != "s0" || a.I != 1 || a.U != 2 || a.B != true || a.F != 3.14 || len(a.L) != 3 {
		t.Fatalf("unexpected defaults: %+v", a)
	}

	// Env overrides
	t.Setenv("S", "X")
	t.Setenv("I", "10")
	t.Setenv("U", "11")
	t.Setenv("B", "false")
	t.Setenv("F", "2.5")
	t.Setenv("L", "[5,6]")

    var b TypesCfg
    if err := ant.SetConfig(&b); err != nil { t.Fatal(err) }
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatalf("SetEffectiveConfig: %v", err)
    }
	if b.S != "X" || b.I != 10 || b.U != 11 || b.B != false || b.F != 2.5 || len(b.L) != 2 || b.L[0] != 5 || b.L[1] != 6 {
		t.Fatalf("unexpected env overrides: %+v", b)
	}
}

func TestEnvParseErrors(t *testing.T) {
	type Cfg struct {
		I int     `env:"I"`
		U uint    `env:"U"`
		B bool    `env:"B"`
		F float64 `env:"F"`
		L []int   `env:"L"`
	}
	ant := &AntConfig{}

	cases := []struct {
		key string
		val string
	}{
		{"I", "fast"},
		{"U", "nope"},
		{"B", "maybe"},
		{"F", "nanx"},
		{"L", "[1, a]"},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			t.Setenv(c.key, c.val)
            var cfg Cfg
            if err := ant.SetConfig(&cfg); err != nil { t.Fatal(err) }
            if err := ant.WriteConfigValues(); err == nil {
                t.Fatalf("expected parse error for %s=%q", c.key, c.val)
            }
		})
	}
}

func TestEmptyEnvDoesNotOverride(t *testing.T) {
	type Cfg struct {
		S string `env:"S" default:"def"`
	}
	ant := &AntConfig{}
	t.Setenv("S", "")
    var cfg Cfg
    if err := ant.SetConfig(&cfg); err != nil { t.Fatal(err) }
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatal(err)
    }
	if cfg.S != "def" {
		t.Fatalf("expected default to remain, got %q", cfg.S)
	}
}

func TestUnsupportedEnvType(t *testing.T) {
	type Cfg struct {
		M map[string]string `env:"M"`
	}
	ant := &AntConfig{}
	t.Setenv("M", "{}")
    var cfg Cfg
    if err := ant.SetConfig(&cfg); err != nil { t.Fatal(err) }
    if err := ant.WriteConfigValues(); err == nil {
        t.Fatal("expected unsupported type error for map with env tag")
    }
}

func TestSliceNonIntIgnored(t *testing.T) {
	type Cfg struct {
		S []string `env:"S"`
	}
	ant := &AntConfig{}
	t.Setenv("S", "[\"a\",\"b\"]")
    var cfg Cfg
    if err := ant.SetConfig(&cfg); err != nil { t.Fatal(err) }
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
	if cfg.S != nil {
		t.Fatalf("expected []string to be untouched (nil), got %#v", cfg.S)
	}
}

func TestDotEnvPrecedenceAndParsing(t *testing.T) {
	// Create a temporary .env file
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	content := "" +
		"# comment\n" +
		"S=fromfile\n" +
		"T=\"val with spaces\"\n" +
		"U=123 # inline comment\n" +
		"export V=\"hello\\nworld\"\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	type Cfg struct {
		S string `env:"S" default:"def"`
		T string `env:"T"`
		U string `env:"U"`
		V string `env:"V"`
	}
	var cfg Cfg

	ant := &AntConfig{}
	if err := ant.SetEnvPath(p); err != nil {
		t.Fatalf("SetEnvPath: %v", err)
	}

	// Explicit env should win over .env
	t.Setenv("S", "fromos")

    if err := ant.SetConfig(&cfg); err != nil { t.Fatal(err) }
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatalf("SetValues: %v", err)
    }

	if cfg.S != "fromos" { // OS wins
		t.Fatalf("expected S from OS, got %q", cfg.S)
	}
	if cfg.T != "val with spaces" {
		t.Fatalf("expected T=\"val with spaces\", got %q", cfg.T)
	}
	if cfg.U != "123" {
		t.Fatalf("expected U stripped of inline comment, got %q", cfg.U)
	}
	if cfg.V != "hello\nworld" {
		t.Fatalf("expected V with newline escape, got %q", cfg.V)
	}
}

func TestDotEnvDoesNotOverrideExplicitEmptyEnv(t *testing.T) {
	// .env has a value, but OS env is explicitly set to empty
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte("K=fileval\n"), 0644); err != nil {
		t.Fatal(err)
	}
	type Cfg struct {
		K string `env:"K" default:"def"`
	}
	var cfg Cfg
	ant := &AntConfig{}
	if err := ant.SetEnvPath(p); err != nil {
		t.Fatal(err)
	}
	t.Setenv("K", "") // explicit empty
    if err := ant.SetConfig(&cfg); err != nil { t.Fatal(err) }
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatal(err)
    }
	if cfg.K != "def" {
		t.Fatalf("expected default to remain when OS env is empty, got %q", cfg.K)
	}
}

func TestDotEnvAutoDiscoveryWorkingDir(t *testing.T) {
	// Create temp dir with a .env file and chdir into it
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte("AUTO_KEY=auto\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	type Cfg struct {
		K string `env:"AUTO_KEY" default:"def"`
	}
	var cfg Cfg
    ant := &AntConfig{} // no EnvPath set -> should auto-discover .env in CWD
    if err := ant.SetConfig(&cfg); err != nil { t.Fatal(err) }
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatalf("SetValues: %v", err)
    }
	if cfg.K != "auto" {
		t.Fatalf("expected auto-discovered .env value, got %q", cfg.K)
	}
}

func TestListFlagsWithPrefix(t *testing.T) {
	var cfg TestConfig
	ant := &AntConfig{}
	ant.SetFlagPrefix("config-")
	specs, err := ant.ListFlags(&cfg)
	if err != nil {
		t.Fatalf("ListFlags error: %v", err)
	}
	// Collect names for easy lookup
	got := map[string]string{}
	for _, s := range specs {
		got[s.Name] = s.CLI
	}
	// Expected flags from TestConfig
	expected := map[string]string{
		"secretkey":    "config-secretkey",
		"encrypt":      "config-encrypt",
		"authuser":     "config-authuser",
		"authpassword": "config-authpassword",
	}
	for name, cli := range expected {
		if got[name] != cli {
			t.Fatalf("missing or wrong CLI for %s: got %q want %q", name, got[name], cli)
		}
	}
}

func TestBindFlagSetAndApply(t *testing.T) {
    var cfg TestConfig
    ant := &AntConfig{}
    ant.SetFlagPrefix("config-")
    fs := flag.NewFlagSet("antconfig-test", flag.ContinueOnError)
    if err := ant.SetConfig(&cfg); err != nil {
        t.Fatalf("SetConfig error: %v", err)
    }
    if err := ant.BindConfigFlags(fs); err != nil {
        t.Fatalf("BindFlags error: %v", err)
    }
	if err := fs.Parse([]string{
		"--config-secretkey", "fs-secret",
		"--config-encrypt=false",
		"--config-authuser=fsuser",
		"--config-authpassword", "fspass",
	}); err != nil {
		t.Fatalf("flag parse error: %v", err)
	}
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatalf("SetValues failed: %v", err)
    }
	if cfg.SecretKey != "fs-secret" {
		t.Fatalf("expected SecretKey from FlagSet, got %q", cfg.SecretKey)
	}
	if cfg.Database.Encrypt != false {
		t.Fatalf("expected Encrypt=false from FlagSet, got %v", cfg.Database.Encrypt)
	}
	if cfg.Database.Auth.User != "fsuser" {
		t.Fatalf("expected Auth.User from FlagSet, got %q", cfg.Database.Auth.User)
	}
	if cfg.Database.Auth.Password != "fspass" {
		t.Fatalf("expected Auth.Password from FlagSet, got %q", cfg.Database.Auth.Password)
	}
}

func TestNestedPointerInit(t *testing.T) {
	type Inner struct {
		Name string `default:"n"`
		Env  string `env:"ENV_NAME"`
	}
	type Outer struct {
		Inner *Inner
	}
	ant := &AntConfig{}
	t.Setenv("ENV_NAME", "y")
    var o Outer
    if err := ant.SetConfig(&o); err != nil { t.Fatal(err) }
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
	if o.Inner == nil {
		t.Fatal("expected Inner to be initialized")
	}
	if o.Inner.Name != "n" || o.Inner.Env != "y" {
		t.Fatalf("unexpected inner values: %+v", o.Inner)
	}
}

func TestJSONC_ToJSON(t *testing.T) {
	src := []byte(`// top comment
{
  "a": 1, // inline
  /* block */
  "b": "text // not comment",
  "arr": [1,2,],
  "obj": {
    "k": "v",
  },
}
`)
	out := ToJSON(src)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal ToJSON output failed: %v\n%s", err, string(out))
	}
	if m["a"].(float64) != 1 {
		t.Fatalf("expected a=1, got %v", m["a"])
	}
	if m["b"].(string) != "text // not comment" {
		t.Fatalf("expected b preserved, got %q", m["b"])
	}
	arr, ok := m["arr"].([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("expected arr length 2, got %#v", m["arr"])
	}
	obj, ok := m["obj"].(map[string]any)
	if !ok || obj["k"].(string) != "v" {
		t.Fatalf("expected obj.k=v, got %#v", m["obj"])
	}
}

func TestJSONC_ToJSONInPlace(t *testing.T) {
	src := []byte(`{"x":1,}//c`)
	out := ToJSONInPlace(src)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal ToJSONInPlace output failed: %v\n%s", err, string(out))
	}
	if m["x"].(float64) != 1 {
		t.Fatalf("expected x=1, got %v", m["x"])
	}
}
