package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	// create flag for --verbose / -v and --help / -h
	flag.Bool("verbose", false, "Enable verbose output")
	flag.Bool("v", false, "(alias for --verbose)")
	flag.Bool("help", false, "Show help message")
	flag.Bool("h", false, "(alias for --help)")

	flag.Usage = Usage
	flag.Parse()

	// check if help flag is set
	_, ok := getFlagValue[bool]("help")
	_, ok2 := getFlagValue[bool]("h")
	if (ok) || (ok2) {
		flag.Usage()
		os.Exit(0)
	}
}

func Usage() {
    fmt.Println("Usage: antconfig [options]")
	fmt.Println("Options:")
	flag.PrintDefaults()
}

func getFlagValue[T any](name string) (T, bool) {
	f := flag.Lookup(name)
	if f == nil {
		var zero T
		return zero, false
	}
	// Check if the flag value can be converted to the desired type
	switch v := f.Value.(flag.Getter).Get().(type) {
	case string:
		if val, ok := any(v).(T); ok {
			return val, true
		}
	case int:
		if val, ok := any(v).(T); ok {
			return val, true
		}
	case bool:
		if val, ok := any(v).(T); ok {
			return val, true
		}
		// Add other types as needed (e.g., *int64, *float64)
	}
	var zero T
	return zero, false
}
