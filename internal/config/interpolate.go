package config

import (
	"fmt"
	"os"
	"regexp"
)

// envPattern matches ${env:VAR} where VAR is an upper-snake identifier. The anchor
// to [A-Z_][A-Z0-9_]* means lowercase names or names with spaces never match.
var envPattern = regexp.MustCompile(`\$\{env:([A-Z_][A-Z0-9_]*)\}`)

// ResolveEnvInterpolation replaces every ${env:VAR} occurrence in raw with
// os.Getenv(VAR). It returns an error if any referenced env var is unset — there
// is no silent fallback to "". Used by the YAML parser for non-Secret string fields
// (Params); Secret fields go through secrets.Secret.UnmarshalYAML instead.
func ResolveEnvInterpolation(raw string) (string, error) {
	var firstErr error
	out := envPattern.ReplaceAllStringFunc(raw, func(match string) string {
		name := envPattern.FindStringSubmatch(match)[1]
		v, ok := os.LookupEnv(name)
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("env var %q referenced but not set", name)
			}
			return match
		}
		return v
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}
