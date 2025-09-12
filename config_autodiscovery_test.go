package antconfig

import (
    "os"
    "path/filepath"
    "testing"
)

func TestConfigAutoDiscovery_Upwards(t *testing.T) {
    root := t.TempDir()
    // Write config.jsonc in root
    cfgPath := filepath.Join(root, "config.jsonc")
    content := []byte(`{
        "A": "cfgA",
        "I": 2,
    }`)
    if err := os.WriteFile(cfgPath, content, 0644); err != nil { t.Fatal(err) }

    // Create child directory and chdir into it
    child := filepath.Join(root, "child")
    if err := os.Mkdir(child, 0755); err != nil { t.Fatal(err) }
    cwd, _ := os.Getwd()
    defer os.Chdir(cwd)
    if err := os.Chdir(child); err != nil { t.Fatal(err) }

    type Cfg struct {
        A string `default:"defA"`
        I int    `default:"1"`
    }
    var cfg Cfg
    ant := &AntConfig{} // no ConfigPath set -> should auto-discover upwards
    if err := ant.SetValues(&cfg); err != nil { t.Fatalf("SetValues: %v", err) }

    if cfg.A != "cfgA" || cfg.I != 2 {
        t.Fatalf("expected auto-discovered config applied, got %+v", cfg)
    }
}

