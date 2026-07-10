package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gdcs-dev/vcpe/controlplane/internal/runtimeinit/servicecmd"
)

func main() {
	err := servicecmd.Run(context.Background(), servicecmd.Config{
		Service:     "event-sink",
		DefaultExec: []string{"/usr/local/bin/entrypoint.sh"},
	}, os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
