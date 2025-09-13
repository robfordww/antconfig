package antconfig

import (
	"os"
	"path/filepath"
	"testing"
)

// Validates precedence:
// 1) defaults
// 2) config file (JSON/JSONC)
// 3) .env file
// 4) OS env vars
// 5) flags
func TestPrecedence_ConfigEnvDotenvFlags(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "app.config.jsonc")
	envPath := filepath.Join(dir, ".env")

	// Config JSONC with comments and trailing comma
	cfgContent := []byte(`{
        // from config file
        "A": "cfgA",
        "B": "cfgB",
        "I": 2,
    }`)
	if err := os.WriteFile(cfgPath, cfgContent, 0644); err != nil {
		t.Fatal(err)
	}

	// .env overrides config (when OS env not set)
	if err := os.WriteFile(envPath, []byte("ACFG_A=dotenvA\nACFG_B=dotenvB\nACFG_I=3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	type Cfg struct {
		A string `default:"defA" env:"ACFG_A" flag:"a"`
		B string `default:"defB" env:"ACFG_B"`
		I int    `default:"1" env:"ACFG_I" flag:"i"`
	}

    var cfg Cfg
    ant := &AntConfig{}
	if err := ant.SetConfigPath(cfgPath); err != nil {
		t.Fatalf("SetConfigPath: %v", err)
	}
	if err := ant.SetEnvPath(envPath); err != nil {
		t.Fatalf("SetEnvPath: %v", err)
	}

	// OS env overrides .env
	t.Setenv("ACFG_A", "OS")

	// First pass: without flags, verify defaults < config < .env < OS env
    if err := ant.SetConfig(&cfg); err != nil { t.Fatalf("SetConfig: %v", err) }
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatalf("SetValues: %v", err)
    }
	if cfg.A == "defA" || cfg.B == "defB" || cfg.I == 1 {
		t.Fatalf("config file should override defaults: %+v", cfg)
	}
	if cfg.B != "dotenvB" {
		t.Fatalf("expected B from .env over config, got %q", cfg.B)
	}
	if cfg.A != "OS" {
		t.Fatalf("expected A from OS env over .env, got %q", cfg.A)
	}

	// Second pass: with flags, verify flags override everything
    cfg = Cfg{}
    if err := ant.SetConfig(&cfg); err != nil { t.Fatalf("SetConfig: %v", err) }
    ant.SetFlagArgs([]string{"--a=FLAG", "--i", "5"})
    if err := ant.WriteConfigValues(); err != nil {
        t.Fatalf("SetValues (flags): %v", err)
    }
	if cfg.A != "FLAG" || cfg.I != 5 || cfg.B != "dotenvB" {
		t.Fatalf("expected flags to win (A, I) and .env to remain for B, got %+v", cfg)
	}
}
