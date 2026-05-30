package main

import (
	"fmt"
	"os"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// buildClient returns a Coolify client and whether it is online. With neither URL nor
// token set, it returns online=false (offline plan). A half-configured pair is an error.
func buildClient(flagURL string) (*coolify.Client, bool, error) {
	url := flagURL
	if url == "" {
		url = os.Getenv("COOLIFY_API_URL")
	}
	_, hasToken := os.LookupEnv("COOLIFY_API_TOKEN")
	switch {
	case url == "" && !hasToken:
		return nil, false, nil
	case url == "" || !hasToken:
		return nil, false, fmt.Errorf("set both a Coolify URL (--coolify-url or COOLIFY_API_URL) and COOLIFY_API_TOKEN, or neither for an offline plan")
	}
	tok, err := secrets.NewFromEnv("COOLIFY_API_TOKEN")
	if err != nil {
		return nil, false, err
	}
	opts := coolify.Options{BaseURL: url, Token: tok}
	if id := os.Getenv("CF_ACCESS_CLIENT_ID"); id != "" {
		sec, sErr := secrets.NewFromEnv("CF_ACCESS_CLIENT_SECRET")
		if sErr != nil {
			return nil, false, sErr
		}
		opts.CFAccessClientID = id
		opts.CFAccessClientSecret = sec
	}
	c, err := coolify.NewClient(opts)
	return c, true, err
}
