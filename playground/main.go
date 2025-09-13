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
	Host string `default:"localhost" env:"CONFIG_HOST"`
	Port int    `default:"8080" env:"CONFIG_PORT"`
	SC   SecretKey
}

func main() {

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	// regular application flags
	var cfg Config
	ac := antconfig.New().MustSetConfig(&cfg)
	ac.SetFlagPrefix("config-")
	ac.MustBindConfigFlags(fs)
	loc, err := antconfig.LocateFromExeUp("config_test.jsonc")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	ac.SetConfigPath(loc)

	fs.Bool("verbose", false, "Enable verbose output")
	fs.Bool("v", false, "(alias for --verbose)")
	fs.Bool("help", false, "Show help message")
	fs.Bool("h", false, "(alias for --help)")

	fs.Usage = func() { Usage(*fs, ac) }

	fs.Parse(os.Args[1:])

	if err := ac.WriteConfigValues(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// if help or h flag is set, print usage and exit
	helpset := fs.Lookup("help").Value.String() == "true" ||
		fs.Lookup("h").Value.String() == "true"
	if helpset {
		fs.Usage()
		os.Exit(0)
	}

	fmt.Printf("Config: %#v\n %#v\n", cfg, cfg.SC)
}

func Usage(fs flag.FlagSet, ac *antconfig.AntConfig) {
	fmt.Println("Usage: antapp [options]")
	fmt.Println("Options:")
	fs.PrintDefaults()
	fmt.Println()
	fmt.Print(ac.EnvHelpString())
}
