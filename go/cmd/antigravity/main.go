package main

import (
	"fmt"
	"os"

	"github.com/antigravity/api-proxy/internal/cmd"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	cmd.Version = Version
	cmd.BuildTime = BuildTime

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
