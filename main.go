package main

import (
	"os"

	"github.com/RunOnYourOwn/track/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
