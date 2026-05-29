package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
)

func appManifest(name, project, env string) string {
	return "api_version: iac-coolify/v1\n" +
		"kind: Application\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		"  project: " + project + "\n" +
		"  environment: " + env + "\n" +
		"spec:\n" +
		"  build_pack: dockerimage\n" +
		"  image:\n" +
		"    name: registry/" + name + "\n" +
		"    tag: v1\n" +
		"  destination:\n" +
		"    server: localhost\n" +
		"    network: coolify\n" +
		"  fqdn: https://" + name + ".example.com\n" +
		"  port: 8000\n"
}

func projectManifest(name string) string {
	return "api_version: iac-coolify/v1\nkind: Project\nmetadata:\n  name: " + name + "\nspec:\n  description: test\n"
}

func envManifest(name, project string) string {
	return "api_version: iac-coolify/v1\nkind: Environment\nmetadata:\n  name: " + name +
		"\n  project: " + project + "\nspec:\n  description: test\n"
}

func serviceManifest(name, project, env string) string {
	return "api_version: iac-coolify/v1\nkind: Service\nmetadata:\n  name: " + name +
		"\n  project: " + project + "\n  environment: " + env +
		"\nspec:\n  destination:\n    server: localhost\n  type: glance\n"
}

// multiEnvDir writes a fixture spanning two environments and every environment-scoped kind,
// so the --env filter can be exercised end to end. Layout per environment: an Environment, an
// Application, plus a Service and Database in staging only.
func multiEnvDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"project.yaml":     projectManifest("beenaire"),
		"env-staging.yaml": envManifest("staging", "beenaire"),
		"env-prod.yaml":    envManifest("production", "beenaire"),
		"app-back.yaml":    appManifest("back-office", "beenaire", "staging"),
		"app-api.yaml":     appManifest("api", "beenaire", "production"),
		"svc-mon.yaml":     serviceManifest("monitoring", "beenaire", "staging"),
		"db-pg.yaml":       dbManifest("pg", "beenaire", "staging", "postgresql", "postgres:18-alpine"),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func applyDryRun(t *testing.T, dir string, args ...string) applyOutput {
	t.Helper()
	clearCoolifyEnv(t)
	full := append([]string{"apply", dir, "--dry-run", "--output=json"}, args...)
	out, err := runCmd(t, full...)
	if err != nil {
		t.Fatalf("apply %v: %v\n%s", args, err, out)
	}
	var got applyOutput
	if jErr := json.Unmarshal([]byte(out), &got); jErr != nil {
		t.Fatalf("parse apply json: %v\n%s", jErr, out)
	}
	return got
}

func wantExit2(t *testing.T, out string, err error, msgSubstr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error, got success:\n%s", out)
	}
	var ec exitCoder
	if !errors.As(err, &ec) || ec.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %v", err)
	}
	if !strings.Contains(out, msgSubstr) {
		t.Errorf("expected message %q, got:\n%s", msgSubstr, out)
	}
}

func TestApply_EnvFilter_ProjectAndEnvironmentKinds(t *testing.T) {
	dir := multiEnvDir(t)
	// Staging selects the project (cross-environment), the staging environment, the staging
	// application and the staging service — but not the production environment or application.
	got := applyDryRun(t, dir, "--env", "staging")
	if got.ToAdd != 5 {
		t.Errorf("--env staging to_add = %d, want 5 (project + env staging + app + service + db)\n%v", got.ToAdd, got.Operations)
	}
	joined := strings.Join(got.Operations, "\n")
	for _, want := range []string{"Project/beenaire", "Environment/beenaire/staging", "back-office", "monitoring", "Database/beenaire/staging/pg"} {
		if !strings.Contains(joined, want) {
			t.Errorf("staging selection missing %q:\n%s", want, joined)
		}
	}
	for _, absent := range []string{"Environment/beenaire/production", "/api"} {
		if strings.Contains(joined, absent) {
			t.Errorf("staging selection must not contain %q:\n%s", absent, joined)
		}
	}
}

func TestApply_EnvFilter_NoFilterAndUnion(t *testing.T) {
	dir := multiEnvDir(t)
	if got := applyDryRun(t, dir); got.ToAdd != 7 {
		t.Errorf("no filter to_add = %d, want 7", got.ToAdd)
	}
	union := applyDryRun(t, dir, "--env", "staging", "--env", "production")
	if union.ToAdd != 7 {
		t.Errorf("union to_add = %d, want 7 (every resource)", union.ToAdd)
	}
}

func TestApply_TargetEnvComposition(t *testing.T) {
	dir := multiEnvDir(t)
	// target alone selects the named resource regardless of environment.
	if got := applyDryRun(t, dir, "--target", "back-office"); got.ToAdd != 1 {
		t.Errorf("--target back-office to_add = %d, want 1", got.ToAdd)
	}
	// target + matching env still selects it.
	if got := applyDryRun(t, dir, "--target", "back-office", "--env", "staging"); got.ToAdd != 1 {
		t.Errorf("--target back-office --env staging to_add = %d, want 1", got.ToAdd)
	}
	// target + a service-bearing env exercises the service kind under composition.
	if got := applyDryRun(t, dir, "--target", "monitoring", "--env", "staging"); got.ToAdd != 1 {
		t.Errorf("--target monitoring --env staging to_add = %d, want 1", got.ToAdd)
	}
	// target + non-matching env matches nothing and must fail loudly.
	clearCoolifyEnv(t)
	out, err := runCmd(t, "apply", dir, "--dry-run", "--output=json", "--target", "back-office", "--env", "production")
	wantExit2(t, out, err, "no resources match --target=back-office --env=production")
}

func TestApply_EnvFilter_UnknownEnvRejected(t *testing.T) {
	dir := multiEnvDir(t)
	clearCoolifyEnv(t)
	out, err := runCmd(t, "apply", dir, "--dry-run", "--output=json", "--env", "preview")
	wantExit2(t, out, err, `environment "preview" matches no resources`)
}

func TestPlan_EnvFilter_ApplicationAndDatabaseKinds(t *testing.T) {
	dir := multiEnvDir(t)
	clearCoolifyEnv(t)
	// Plan covers applications and databases; staging holds back-office (app) and pg (db).
	out, err := runCmd(t, "plan", dir, "--output=json", "--env", "staging")
	if err != nil {
		t.Fatalf("plan --env staging: %v\n%s", err, out)
	}
	var got plan.Plan
	if jErr := json.Unmarshal([]byte(out), &got); jErr != nil {
		t.Fatalf("parse plan json: %v\n%s", jErr, out)
	}
	names := map[string]bool{}
	for _, it := range got.Items {
		names[it.Kind+"/"+it.Name] = true
	}
	if len(got.Items) != 2 || !names["Application/back-office"] || !names["Database/pg"] {
		t.Errorf("staging plan items = %v, want Application/back-office + Database/pg", got.Items)
	}
}

func TestPlan_TargetEnvComposition(t *testing.T) {
	dir := multiEnvDir(t)
	clearCoolifyEnv(t)
	// --target now exists on plan and composes with --env.
	out, err := runCmd(t, "plan", dir, "--output=json", "--target", "pg", "--env", "staging")
	if err != nil {
		t.Fatalf("plan --target pg --env staging: %v\n%s", err, out)
	}
	var got plan.Plan
	if jErr := json.Unmarshal([]byte(out), &got); jErr != nil {
		t.Fatalf("parse plan json: %v\n%s", jErr, out)
	}
	if len(got.Items) != 1 || got.Items[0].Name != "pg" {
		t.Errorf("plan items = %v, want only Database/pg", got.Items)
	}
}

func TestPlan_EnvFilter_UnknownEnvRejected(t *testing.T) {
	dir := multiEnvDir(t)
	clearCoolifyEnv(t)
	out, err := runCmd(t, "plan", dir, "--output=json", "--env", "preview")
	wantExit2(t, out, err, `environment "preview" matches no resources`)
}

func TestDestroy_EnvFilter(t *testing.T) {
	dir := multiEnvDir(t)
	clearCoolifyEnv(t)
	out, err := runCmd(t, "destroy", dir, "--dry-run", "--output=json", "--env", "staging")
	if err != nil {
		t.Fatalf("destroy --env staging: %v\n%s", err, out)
	}
	var got applyOutput
	if jErr := json.Unmarshal([]byte(out), &got); jErr != nil {
		t.Fatalf("parse destroy json: %v\n%s", jErr, out)
	}
	if got.ToDestroy != 5 {
		t.Errorf("destroy --env staging to_destroy = %d, want 5 (project + env staging + app + service + db)\n%v", got.ToDestroy, got.Operations)
	}
}

func TestApply_DatabaseFilterByEnv(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"project.yaml":  projectManifest("beenaire"),
		"env-stg.yaml":  envManifest("staging", "beenaire"),
		"env-prod.yaml": envManifest("production", "beenaire"),
		"pg.yaml":       dbManifest("pg", "beenaire", "staging", "postgresql", "postgres:18-alpine"),
		"cache.yaml":    dbManifest("cache", "beenaire", "staging", "redis", "redis:7-alpine"),
		"prod-db.yaml":  dbManifest("prod-db", "beenaire", "production", "postgresql", "postgres:18-alpine"),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// staging holds two databases plus the staging environment and the project; production
	// holds one database plus its environment and the (cross-env) project.
	staging := applyDryRun(t, dir, "--env", "staging")
	gotStaging := countDatabaseOps(staging.Operations)
	if gotStaging != 2 {
		t.Errorf("--env staging database ops = %d, want 2 (pg + cache)\n%v", gotStaging, staging.Operations)
	}
	production := applyDryRun(t, dir, "--env", "production")
	gotProd := countDatabaseOps(production.Operations)
	if gotProd != 1 {
		t.Errorf("--env production database ops = %d, want 1 (prod-db)\n%v", gotProd, production.Operations)
	}
}

func countDatabaseOps(ops []string) int {
	n := 0
	for _, op := range ops {
		if strings.Contains(op, "Database/") {
			n++
		}
	}
	return n
}

func TestDestroy_EnvFilter_UnknownEnvRejected(t *testing.T) {
	dir := multiEnvDir(t)
	clearCoolifyEnv(t)
	out, err := runCmd(t, "destroy", dir, "--dry-run", "--output=json", "--env", "preview")
	wantExit2(t, out, err, `environment "preview" matches no resources`)
}
