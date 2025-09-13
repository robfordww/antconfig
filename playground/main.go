package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/robfordww/antconfig"
)

type SecretKey struct {
	Key string `default:"secretkey123" env:"SEC" flag:"sec"`
}

type Config struct {
	Host string `default:"localhost" env:"CONFIG_HOST"`
	Port int    `default:"8080" env:"CONFIG_PORT"`
	SC   *SecretKey
}

func main() {

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	// regular application flags
	fs.Bool("verbose", false, "Enable verbose output")
	fs.Bool("v", false, "(alias for --verbose)")
	fs.Bool("help", false, "Show help message")
	fs.Bool("h", false, "(alias for --help)")

	var cfg Config
	ac := antconfig.AntConfig{}
	// Provide the config reference, then register config-derived flags
	if err := ac.SetConfig(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error setting config: %v\n", err)
		os.Exit(1)
	}
	if err := ac.BindConfigFlags(fs); err != nil {
		fmt.Fprintf(os.Stderr, "error binding config flags: %v\n", err)
		os.Exit(1)
	}

	fs.Usage = func() { Usage(*fs) }

	fs.Parse(os.Args[1:])

	if err := ac.WriteConfigValues(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Config: %#v\n %v\n", cfg, cfg.SC)

	// if help or h flag is set, print usage and exit
	helpset := fs.Lookup("help").Value.String() == "true" ||
		fs.Lookup("h").Value.String() == "true"
	if helpset {
		Usage(*fs)
		os.Exit(0)
	}
}

func Usage(fs flag.FlagSet) {
	fmt.Println("Usage: antapp [options]")
	fmt.Println("Options:")
	fs.PrintDefaults()
}
