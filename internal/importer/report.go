package importer

import (
	"fmt"
	"sort"
	"strings"
)

// completeCount returns how many imported applications round-tripped to a valid manifest.
func (r Report) completeCount() int {
	n := 0
	for _, a := range r.Applications {
		if a.Complete {
			n++
		}
	}
	return n
}

// RenderText formats the import outcome for the operator: counts, the services the API
// cannot reconstruct, the applications needing a manual field, and the env vars to populate.
func (r Report) RenderText() string {
	var b strings.Builder
	complete := r.completeCount()
	fmt.Fprintf(&b, "imported %d application(s) (%d complete, %d partial), %d database(s)\n",
		len(r.Applications), complete, len(r.Applications)-complete, len(r.Databases))

	if len(r.ServicesSkipped) > 0 {
		fmt.Fprintf(&b, "services skipped (Coolify API does not expose their spec — coollabsio/coolify#10449): %s\n",
			strings.Join(r.ServicesSkipped, ", "))
	}
	r.renderPartials(&b)
	r.renderSecrets(&b)
	if r.Dropped > 0 {
		fmt.Fprintf(&b, "dropped/unmapped: %d (no matching environment — see warnings above)\n", r.Dropped)
	}
	return b.String()
}

func (r Report) renderPartials(b *strings.Builder) {
	var partial []AppResult
	for _, a := range r.Applications {
		if !a.Complete {
			partial = append(partial, a)
		}
	}
	if len(partial) == 0 {
		return
	}
	sort.Slice(partial, func(i, j int) bool { return partial[i].Name < partial[j].Name })
	fmt.Fprintln(b, "partial applications (complete manually before apply):")
	for _, a := range partial {
		fmt.Fprintf(b, "  - %s (%s): %s\n", a.Name, a.Environment, a.Reason)
	}
}

func (r Report) renderSecrets(b *strings.Builder) {
	keys := append([]string{}, r.EnvKeys...)
	keys = append(keys, r.PasswordEnvs...)
	keys = uniqueSorted(keys)
	if len(keys) == 0 {
		return
	}
	fmt.Fprintln(b, "secrets referenced — populate your .env with these keys:")
	for _, k := range keys {
		fmt.Fprintf(b, "  - %s\n", k)
	}
}
