package main

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/tui"
)

// connectFromWizard returns the connector the wizard calls on submit. It writes the entered
// values to the environment and rebuilds the client through buildClient, so the token becomes a
// ${env:} secret via secrets.NewFromEnv — never a hardcoded Secret literal — then tests the
// connection with a light read. ctx bounds that test request. The error from a failed test
// carries no credential: the token is a Secret, unreachable by any format verb.
func connectFromWizard(ctx context.Context) tui.ConnectFunc {
	return func(in tui.ConnectInput) (*coolify.Client, error) {
		env := [][2]string{
			{"COOLIFY_API_URL", in.URL},
			{"COOLIFY_API_TOKEN", in.Token},
		}
		if in.CFAccessID != "" {
			env = append(env,
				[2]string{"CF_ACCESS_CLIENT_ID", in.CFAccessID},
				[2]string{"CF_ACCESS_CLIENT_SECRET", in.CFAccessSecret},
			)
		}
		for _, kv := range env {
			if err := os.Setenv(kv[0], kv[1]); err != nil {
				return nil, fmt.Errorf("set %s: %w", kv[0], err)
			}
		}
		client, online, err := buildClient("")
		if err != nil {
			return nil, err
		}
		if !online {
			return nil, fmt.Errorf("a Coolify URL and API token are both required")
		}
		if _, err := client.ListProjects(ctx); err != nil {
			return nil, fmt.Errorf("connection test failed: %w", err)
		}
		return client, nil
	}
}

// connectInteractively runs the connection wizard and returns the connected client, or nil if
// the user quit before a connection succeeded. It is used when explore starts with no
// credentials in the environment.
func connectInteractively(ctx context.Context) (*coolify.Client, error) {
	final, err := tea.NewProgram(
		tui.NewConnectModel(connectFromWizard(ctx)),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
	).Run()
	if err != nil {
		return nil, err
	}
	model, ok := final.(tui.ConnectModel)
	if !ok {
		return nil, fmt.Errorf("explore: unexpected wizard model %T", final)
	}
	return model.Client(), nil
}
