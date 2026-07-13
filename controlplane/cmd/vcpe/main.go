package main

import (
	"fmt"
	"os"

	"github.com/gdcs-dev/vcpe/controlplane/internal/app"
)

// version is the build-time version string. It defaults to "dev" and is
// overridden by -ldflags "-X main.version=<tag>" during release builds.
var version = "dev"

func main() {
	if err := app.ExecuteCLI(os.Args[0], os.Args[1:], version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
