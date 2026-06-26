package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gdcs-dev/vcpe/controlplane/internal/runtimeinit/servicecmd"
)

func main() {
	err := servicecmd.Run(context.Background(), servicecmd.Config{
		Service:     "client",
		DefaultExec: []string{"tail", "-f", "/dev/null"},
	}, os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
