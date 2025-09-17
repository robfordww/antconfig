![AntConfig](assets/antconfig.png)

# AntConfig

[![Go Reference](https://pkg.go.dev/badge/github.com/robfordww/antconfig.svg)](https://pkg.go.dev/github.com/robfordww/antconfig)
[![Go Report Card](https://goreportcard.com/badge/github.com/robfordww/antconfig)](https://goreportcard.com/report/github.com/robfordww/antconfig)

AntConfig is a small, zero-dependency Go configuration library focused on simplicity, clarity, and predictable precedence. Configuration is defined through tagged structs, which can be overridden by environment variables, a .env file, or command-line flags. Optional configuration files are supported in JSON or JSONC format only. Unlike many other configuration libraries that include support for TOML, YAML, and extensive feature sets, AntConfig is opinionated: it keeps things minimal, simple, and free of external dependencies.

## Why Choose AntConfig for Go Configuration

- Opinionated, zero-dependency design keeps binaries small and secure.
- Works out of the box with tagged structs, env vars, JSON/JSONC files, and CLI flags.
- Automatic config discovery and `.env` loading streamline deployment workflows.
- Type-safe parsing for core Go types avoids reflection surprises at runtime.

## Features

- Zero dependencies: uses only the Go standard library.
- JSON and JSONC: helpers to strip comments and trailing commas for JSONC.
- Tag-based configuration: `default:"…"` and `env:"ENV_NAME"` on struct fields.
- Nested structs supported: including pointer fields (auto-initialized when needed).
- Type-safe env parsing: string, int/uint, bool, float64, and `[]int` from JSON.
- Supports .env files
- Discovery helpers: locate config file by walking upward from CWD or executable.

## Use Cases

- Bootstrapping new Go microservices with minimal config wiring.
- Shipping CLIs where predictable flag and env precedence is critical.
- Migrating from heavier configuration stacks (e.g., Viper) to a lighter runtime.
- Embedding configuration into serverless functions or containers with strict size budgets.


## Status and Precedence

Current precedence when applying configuration values:

1) Defaults from struct tags (`default:"…"`)
2) Configuration file (.json or .jsonc). If no path is set via `SetConfigPath`, AntConfig auto-discovers `config.jsonc` or `config.json` starting from the current working directory and walking upward.
3) .env file (when `SetEnvPath` is used)
4) Environment variables (`env:"NAME"`) — override .env
5) Command line flags (`flag:"name"`) — highest priority

## Quick Start

Install via `go get` (Go modules):

```bash
go get github.com/robfordww/antconfig@latest
```

```go
package main

import (
    "flag"
    "fmt"
    "os"

    "github.com/robfordww/antconfig"
)

type SecretKey struct {
    Key string `default:"secretkey123" env:"SEC" flag:"sec" desc:"Secret key for encryption"`
}

type Config struct {
    Host string    `default:"localhost" env:"CONFIG_HOST"`
    Port int       `default:"8080" env:"CONFIG_PORT"`
    SC   SecretKey
}

func main() {
    fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

    var cfg Config
    ac := antconfig.New().MustSetConfig(&cfg)
    ac.SetFlagPrefix("config-")            // optional flag prefix
    ac.MustBindConfigFlags(fs)             // register flags from struct tags

    // Optional: add your own app flags
    fs.Bool("verbose", false, "Enable verbose output")

    // Show env help after flag defaults
    fs.Usage = func() {
        fs.PrintDefaults()
        fmt.Print("\n" + ac.EnvHelpString())
    }

    _ = fs.Parse(os.Args[1:])             // parse CLI args

    // Apply: defaults -> config file -> .env -> env -> flags
    if err := ac.WriteConfigValues(); err != nil {
        fmt.Fprintln(os.Stderr, "error:", err)
        os.Exit(1)
    }

    fmt.Printf("Config: %#v\n", cfg)
}
```

Behavior shown above is verified in tests: defaults are set first, then env
vars (if non-empty) override them. Empty env values do not override defaults.

## JSONC Support

The `jsonc.go` helper lets you read JSONC (comments + trailing commas) and
turn it into strict JSON before unmarshaling.

```go
data, err := os.ReadFile("config_test.jsonc")
if err != nil { /* handle */ }

jsonBytes := antconfig.ToJSON(data) // or ToJSONInPlace(data)
if err := json.Unmarshal(jsonBytes, &cfg); err != nil { /* handle */ }
```

An example JSONC file is included at `config_test.jsonc`.

## Config Discovery Helpers

Two helpers return a config file path by walking parent directories up to a
limit (10 levels):

- `antconfig.LocateFromWorkingDir(filename)`
- `antconfig.LocateFromExe(filename)`

Both return the first match travering upwards from the directory, otherwise `ErrConfigNotFound` is returned.

## API Overview (package `antconfig`)

- `type AntConfig` (fields unexported)
  - `SetEnvPath(path string) error`: set `.EnvPath` and validate the file exists. When set, `.env` is loaded and variables are added to the process environment only if they are not already set. If `EnvPath` is not set, AntConfig auto-discovers a `.env` in the current working directory.
  - `SetConfigPath(path string) error`: set `.ConfigPath` and validate it exists.
  - `WriteConfigValues() error`: apply defaults, config file (JSON/JSONC), .env, env, then flag overrides to the config passed via `SetConfig`.
  - `SetFlagArgs(args []string)`: provide explicit CLI args (defaults to `os.Args[1:]`).
  - `SetFlagPrefix(prefix string)`: set optional prefix used for generated CLI flags.
  - `ListFlags(cfg any) ([]FlagSpec, error)`: return available flags with names and types.
  - `SetConfig(&cfg) error`: provide the config pointer for reflection when binding flags.
  - `MustSetConfig(&cfg) *AntConfig`: like `SetConfig` but panics on error and returns the receiver for chaining.
  - `BindConfigFlags(fs *flag.FlagSet) error`: register flags derived from your config onto a provided `FlagSet` (and bind it for later reads).

- Struct tags on `cfg` fields
  - `default:"…"`: default value used when field is zero-value.
  - `env:"ENV_NAME"`: if present and non-empty, overrides the field with a parsed value.
  - `flag:"name"`: if present, allows `--name value` (or `--name=value`) to override the field. When `SetFlagPrefix("config-")` is set, use `--config-name` instead.
  - `desc:"…"`: optional description used as usage text when registering flags via `BindConfigFlags` and shown in env help.

## Dynamic Flag Usage

You can build CLI usage dynamically from your config struct. For example:

```go
var cfg AppConfig
ant := antconfig.New().MustSetConfig(&cfg)
ant.SetFlagPrefix("config-") // optional
flags, _ := ant.ListFlags(&cfg)
fmt.Println("Config flags:")
for _, f := range flags {
    fmt.Printf("  --%s  (%s)\n", f.CLI, f.Kind)
}

// Populate a FlagSet and parse
fs := flag.NewFlagSet("myapp", flag.ExitOnError)
if err := ant.BindConfigFlags(fs); err != nil { panic(err) }
_ = fs.Parse(os.Args[1:])
// Apply: defaults -> config file -> .env -> env -> flags (from FlagSet)
if err := ant.WriteConfigValues(); err != nil { panic(err) }
```

## Notes

- Nested structs and pointers to structs are traversed and initialized as needed.
- Empty env values do not override defaults.

## Playground

A small playground command included under `cmd/`. Use it as a experimental testingground.

Build and run:

```bash
go build -o antapp ./playground
./antapp -config-host=localhost
```

## Testing

Run all tests:

```bash
go test ./...
```

The tests exercise defaults, env overrides, pointer initialization for nested
structs, discovery helpers, and JSONC parsing.

---

Feel free to open an issue or PR if you have any suggestions.
