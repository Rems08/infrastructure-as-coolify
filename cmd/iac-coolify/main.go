// Command iac-coolify is the declarative Infrastructure-as-Code CLI for Coolify v4.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
)

func main() {
	os.Exit(run())
}

// run executes the root command and returns the process exit code, keeping os.Exit
// out of any deferred-cleanup scope.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "iac-coolify:", err)
		return 1
	}
	return 0
}
