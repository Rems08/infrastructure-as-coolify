package config

import (
	"math"
	"regexp"
)

// knownSecretPattern matches the canary prefixes of common credential formats. The
// alternatives for eyJ/age1 capture the full token so the reported match clearly shows
// the offending value.
var knownSecretPattern = regexp.MustCompile(
	`ghp_[A-Za-z0-9]{16,}|gho_[A-Za-z0-9]{16,}|ghs_[A-Za-z0-9]{16,}|sk-[A-Za-z0-9]{10,}|AKIA[A-Z0-9]{12,}|xoxb-[A-Za-z0-9-]{10,}|xoxp-[A-Za-z0-9-]{10,}|eyJ[A-Za-z0-9_-]{10,}|age1[a-z0-9]{20,}`,
)

// base64ish recognises tokens made of the alphabet typical of base64/hex secrets.
var base64ish = regexp.MustCompile(`^[A-Za-z0-9+/=_\-]+$`)

const (
	// entropyThreshold is the Shannon entropy (bits/char) above which a sufficiently
	// long opaque token is treated as secret-like.
	entropyThreshold = 4.5
	// minEntropyLen avoids flagging short human-readable values like "production".
	minEntropyLen = 20
)

// DetectSecretLike reports whether value looks like a credential and, if so, a short
// human-readable reason. It is used by `validate --strict` to scan visible `value:`
// fields and suggest migrating to `value_secret:`.
func DetectSecretLike(value string) (reason string, ok bool) {
	if m := knownSecretPattern.FindString(value); m != "" {
		return "matches known secret pattern: " + m, true
	}
	if len(value) >= minEntropyLen && base64ish.MatchString(value) {
		if e := shannonEntropy(value); e >= entropyThreshold {
			return "high entropy opaque token (looks like a secret)", true
		}
	}
	return "", false
}

func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	freq := make(map[rune]float64)
	runes := []rune(s)
	for _, r := range runes {
		freq[r]++
	}
	n := float64(len(runes))
	var e float64
	for _, c := range freq {
		p := c / n
		e -= p * math.Log2(p)
	}
	return e
}
