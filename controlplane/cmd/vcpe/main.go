package main

import (
	"fmt"
	"os"

	"github.com/gdcs-dev/vcpe/controlplane/internal/app"
)

func main() {
	if err := app.ExecuteCLI(os.Args[0], os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
