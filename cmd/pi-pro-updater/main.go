package main

import (
	"fmt"
	"os"

	"github.com/a754962942/pi-pro-cli/internal/updater"
)

func main() {
	if len(os.Args) != 2 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: pi-pro-updater <update-state.json>")
		os.Exit(2)
	}
	if err := updater.RunHelper(os.Args[1]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
