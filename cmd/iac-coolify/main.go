// Command iac-coolify is the declarative Infrastructure-as-Code CLI for Coolify v4.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
)

func main() {
	os.Exit(run())
}

// exitCoder lets a command request a specific process exit code without being treated as
// a runtime error (e.g. `plan --detailed-exitcode` returns 2 when changes are pending).
type exitCoder interface{ ExitCode() int }

// run executes the root command and returns the process exit code, keeping os.Exit
// out of any deferred-cleanup scope.
func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		var ec exitCoder
		if errors.As(err, &ec) {
			return ec.ExitCode() // clean exit code; the command already produced its output
		}
		fmt.Fprintln(os.Stderr, "iac-coolify:", err)
		return 1
	}
	return 0
}
