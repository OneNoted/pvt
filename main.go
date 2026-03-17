package main

import (
	"os"

	"github.com/mirceanton/pvt/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
