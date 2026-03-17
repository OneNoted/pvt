package main

import (
	"os"

	"github.com/OneNoted/pvt/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
