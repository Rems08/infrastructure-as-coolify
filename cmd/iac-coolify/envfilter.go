package main

// matchesEnv reports whether env is in the allow set. An empty allow set means no
// environment filter is active, so every resource matches.
func matchesEnv(allow []string, env string) bool {
	if len(allow) == 0 {
		return true
	}
	for _, e := range allow {
		if e == env {
			return true
		}
	}
	return false
}
