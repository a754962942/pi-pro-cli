package main

import (
	"os"

	"github.com/a754962942/pi-pro-cli/internal/commands"
)

func main() {
	os.Exit(commands.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
