package provision

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// deployDir copies the real production Compose file, Caddyfile and template
// into a fresh directory whose path has spaces and an apostrophe, like many
// Windows profile paths.
func deployDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Laptop Click's deploy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{composeFileName, templateFileName, "Caddyfile"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

type fakeDocker struct {
	resources      map[string][]string
	validateOutput string
	validateErr    error
	calls          []string
	validatedEnv   []map[string]string
}

func (f *fakeDocker) ProjectResources(_ context.Context, project string) ([]string, error) {
	f.calls = append(f.calls, "resources "+project)
	return f.resources[project], nil
}

func (f *fakeDocker) ValidateCompose(_ context.Context, _ string, env map[string]string) (string, error) {
	f.calls = append(f.calls, "validate "+env[keyProject])
	f.validatedEnv = append(f.validatedEnv, env)
	return f.validateOutput, f.validateErr
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no randomness in a dry run") }

func testDeps(docker *fakeDocker, out *bytes.Buffer) Deps {
	return Deps{Random: rand.Reader, Docker: docker, PortFree: func(int) bool { return true }, Out: out}
}

func create(t *testing.T, dir, code, domain string, docker *fakeDocker, out *bytes.Buffer, edit ...func(*Options)) (*Result, error) {
	t.Helper()
	opts := Options{Code: code, Domain: domain, ACMEEmail: "it@flybirds.com", DeployDir: dir}
	for _, e := range edit {
		e(&opts)
	}
	return Create(context.Background(), opts, testDeps(docker, out))
}

func readEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := parseEnv(string(data))
	if err != nil {
		t.Fatal(err)
	}
	return envValues(lines)
}

// snapshot records every file name and content in dir.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = string(data)
	}
	return files
}

func TestValidateCode(t *testing.T) {
	for _, code := range []string{"demo-school", "abc", "a1-b2-c3", "greenvalley", strings.Repeat("a", 32)} {
		if err := ValidateCode(code); err != nil {
			t.Errorf("ValidateCode(%q) = %v, want nil", code, err)
		}
	}
	for _, code := range []string{"", "ab", strings.Repeat("a", 33), "Demo", "demo_school", "-demo", "demo-", "demo--x", "1demo",
		"../etc", "..", "demo/x", `demo\x`, "demo;rm -rf", "demo school", "demo\nx", "dém", "demo.env", "$(id)", "`id`", "demo'x", "ci*"} {
		if err := ValidateCode(code); err == nil {
			t.Errorf("ValidateCode(%q) = nil, want an error", code)
		}
	}
}

func TestNormalizeDomain(t *testing.T) {
	valid := map[string]string{
		"demo-school.flybirds.com": "demo-school.flybirds.com",
		"GreenValley.FlyBirds.com": "greenvalley.flybirds.com",
		"school.example.edu.bd":    "school.example.edu.bd",
		"a.b.c.d.example.org":      "a.b.c.d.example.org",
		"xn--80ak6aa92e.xn--p1ai":  "xn--80ak6aa92e.xn--p1ai",
		"localhost":                "localhost",
		"alpha.localhost":          "alpha.localhost",
	}
	for raw, want := range valid {
		if got, err := NormalizeDomain(raw); err != nil || got != want {
			t.Errorf("NormalizeDomain(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "flybirds", "https://a.flybirds.com", "a.flybirds.com:443", "a.flybirds.com/login", "*.flybirds.com",
		"a..flybirds.com", "-a.flybirds.com", "a-.flybirds.com", "a_b.flybirds.com", "a.flybirds.com.", "127.0.0.1", "::1", "10.0.0.1.",
		"school.123", "a flybirds.com", " a.flybirds.com", "a.flybirds.com\nEVIL=1", "{$HOME}.flybirds.com", "a.flybirds.com}", "a.flybirds.com {",
		"user@a.flybirds.com", "a.flybirds.com#x", "dhaka-school.com\r", "école.flybirds.com",
		strings.Repeat("a", 64) + ".flybirds.com", strings.Repeat("abcdefghi.", 26) + "com"} {
		if got, err := NormalizeDomain(raw); err == nil {
			t.Errorf("NormalizeDomain(%q) = %q, nil; want an error", raw, got)
		}
	}
}

func TestValidateACMEEmail(t *testing.T) {
	for _, email := range []string{"it@flybirds.com", "head.office+tls@school.edu.bd"} {
		if err := ValidateACMEEmail(email); err != nil {
			t.Errorf("ValidateACMEEmail(%q) = %v", email, err)
		}
	}
	for _, email := range []string{"", "it", "it@", "it@example.edu.bd", "it@school.test", "it@flybirds.com\nX=1", "a b@flybirds.com", "it@localhost"} {
		if err := ValidateACMEEmail(email); err == nil {
			t.Errorf("ValidateACMEEmail(%q) = nil, want an error", email)
		}
	}
}

func TestCreateWritesCompleteEnvironmentFile(t *testing.T) {
	dir := deployDir(t)
	var out bytes.Buffer
	result, err := create(t, dir, "demo-school", "Demo-School.FlyBirds.com", &fakeDocker{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Written || result.EnvFile != filepath.Join(dir, "demo-school.env") {
		t.Fatalf("result = %+v", result)
	}
	values := readEnv(t, result.EnvFile)
	templateData, _ := os.ReadFile(filepath.Join(dir, templateFileName))
	templateLines, _ := parseEnv(string(templateData))
	for key := range envValues(templateLines) {
		if strings.TrimSpace(values[key]) == "" {
			t.Errorf("%s has no value", key)
		}
	}
	want := map[string]string{keyProject: "demo-school", keyProfiles: "edge", keyDomain: "demo-school.flybirds.com", keyACMEEmail: "it@flybirds.com", keyWebPort: "8081"}
	for key, value := range want {
		if values[key] != value {
			t.Errorf("%s = %q, want %q", key, values[key], value)
		}
	}
	if raw, err := hex.DecodeString(values[keyDBPassword]); err != nil || len(raw) != dbPasswordSize {
		t.Errorf("database password is not %d random bytes in hex", dbPasswordSize)
	}
	if raw, err := base64.StdEncoding.DecodeString(values[keySigningKey]); err != nil || len(raw) < 32 {
		t.Errorf("signing key is not base64 of at least 32 bytes")
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(result.EnvFile); info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", info.Mode().Perm())
		}
	}
	if !strings.Contains(out.String(), "Next steps") {
		t.Errorf("no next steps in output:\n%s", out.String())
	}
}

func TestLocalDomainNeedsNoCertificateContact(t *testing.T) {
	dir := deployDir(t)
	if _, err := create(t, dir, "trial", "trial.localhost", &fakeDocker{}, &bytes.Buffer{}, func(o *Options) { o.ACMEEmail = "" }); err != nil {
		t.Fatal(err)
	}
	if _, err := create(t, dir, "public", "public.flybirds.com", &fakeDocker{}, &bytes.Buffer{}, func(o *Options) { o.ACMEEmail = "" }); err == nil {
		t.Fatal("a public domain without --acme-email was accepted")
	}
}

func TestTwoSchoolsAreIndependent(t *testing.T) {
	dir := deployDir(t)
	alpha, err := create(t, dir, "alpha", "alpha.flybirds.com", &fakeDocker{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := create(t, dir, "beta", "beta.flybirds.com", &fakeDocker{}, &bytes.Buffer{}, func(o *Options) { o.NoEdge = true })
	if err != nil {
		t.Fatal(err)
	}
	a, b := readEnv(t, alpha.EnvFile), readEnv(t, beta.EnvFile)
	for _, key := range []string{keyDBPassword, keySigningKey, keyProject, keyDomain, keyWebPort} {
		if a[key] == b[key] {
			t.Errorf("both schools share %s", key)
		}
	}
	if a[keyWebPort] != "8081" || b[keyWebPort] != "8082" || b[keyProfiles] != "" {
		t.Errorf("ports %s/%s, beta profiles %q", a[keyWebPort], b[keyWebPort], b[keyProfiles])
	}
	for _, v := range alpha.Volumes {
		if !strings.HasPrefix(v, "alpha_") {
			t.Errorf("alpha volume %s is outside alpha_", v)
		}
		for _, w := range beta.Volumes {
			if v == w || strings.HasPrefix(w, "alpha_") {
				t.Errorf("volume overlap: %s / %s", v, w)
			}
		}
	}
	for _, v := range beta.Volumes {
		if v == "beta_caddy_data" || v == "beta_caddy_config" {
			t.Errorf("beta without the edge lists %s", v)
		}
	}
	if !contains(beta.Volumes, "beta_edge_socket") || contains(beta.Containers, "beta-caddy-1") || !contains(alpha.Containers, "alpha-caddy-1") {
		t.Errorf("resources: alpha %v %v, beta %v %v", alpha.Volumes, alpha.Containers, beta.Volumes, beta.Containers)
	}
	schools, err := List(dir)
	if err != nil || len(schools) != 2 || schools[0].Code != "alpha" || schools[1].Code != "beta" {
		t.Fatalf("List = %+v, %v", schools, err)
	}
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func TestDuplicateProvisioningIsRefused(t *testing.T) {
	dir := deployDir(t)
	if _, err := create(t, dir, "alpha", "alpha.flybirds.com", &fakeDocker{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, dir)
	cases := map[string]func() (*Result, error){
		"same code": func() (*Result, error) {
			return create(t, dir, "alpha", "other.flybirds.com", &fakeDocker{}, &bytes.Buffer{})
		},
		"same domain": func() (*Result, error) {
			return create(t, dir, "gamma", "ALPHA.flybirds.com", &fakeDocker{}, &bytes.Buffer{})
		},
		"same web port": func() (*Result, error) {
			return create(t, dir, "gamma", "gamma.flybirds.com", &fakeDocker{}, &bytes.Buffer{}, func(o *Options) { o.WebPort = 8081 })
		},
		"port busy on this machine": func() (*Result, error) {
			deps := testDeps(&fakeDocker{}, &bytes.Buffer{})
			deps.PortFree = func(int) bool { return false }
			return Create(context.Background(), Options{Code: "gamma", Domain: "gamma.flybirds.com", ACMEEmail: "it@flybirds.com", WebPort: 9000, DeployDir: dir}, deps)
		},
		"same domain in a hand-written file": func() (*Result, error) {
			manual := "COMPOSE_PROJECT_NAME=Manual\nSCHOOL_DOMAIN=Delta.FlyBirds.com\nPOSTGRES_PASSWORD=x\nJWT_SIGNING_KEY_B64=x\nWEB_PORT=8090\n"
			if err := os.WriteFile(filepath.Join(dir, "manual.env"), []byte(manual), 0o600); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(filepath.Join(dir, "manual.env"))
			return create(t, dir, "delta", "delta.flybirds.com", &fakeDocker{}, &bytes.Buffer{})
		},
		"same code in a hand-written file": func() (*Result, error) {
			manual := "COMPOSE_PROJECT_NAME=Epsilon\nSCHOOL_DOMAIN=other.flybirds.com\nPOSTGRES_PASSWORD=x\nJWT_SIGNING_KEY_B64=x\nWEB_PORT=8091\n"
			if err := os.WriteFile(filepath.Join(dir, "manual.env"), []byte(manual), 0o600); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(filepath.Join(dir, "manual.env"))
			return create(t, dir, "epsilon", "epsilon.flybirds.com", &fakeDocker{}, &bytes.Buffer{})
		},
		"existing Docker project": func() (*Result, error) {
			docker := &fakeDocker{resources: map[string][]string{"gamma": {"volume gamma_postgres_data"}}}
			return create(t, dir, "gamma", "gamma.flybirds.com", docker, &bytes.Buffer{})
		},
	}
	for name, run := range cases {
		if _, err := run(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if after := snapshot(t, dir); !equalMaps(before, after) {
		t.Errorf("a refused provisioning changed the deploy directory")
	}
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestSecretsNeverAppearInOutput(t *testing.T) {
	dir := deployDir(t)
	var out bytes.Buffer
	docker := &fakeDocker{}
	result, err := create(t, dir, "alpha", "alpha.flybirds.com", docker, &out)
	if err != nil {
		t.Fatal(err)
	}
	values := readEnv(t, result.EnvFile)
	for _, key := range []string{keyDBPassword, keySigningKey} {
		if strings.Contains(out.String(), values[key]) {
			t.Errorf("%s was printed", key)
		}
	}
	// Docker's own messages are redacted before they are shown.
	echo := &fakeDocker{validateErr: errors.New("exit status 1")}
	echo.validateOutput = "invalid"
	var out2 bytes.Buffer
	deps := testDeps(echo, &out2)
	deps.Docker = echoingDocker{echo}
	_, err = Create(context.Background(), Options{Code: "beta", Domain: "beta.flybirds.com", ACMEEmail: "it@flybirds.com", DeployDir: dir}, deps)
	if err == nil {
		t.Fatal("a rejected Compose configuration was accepted")
	}
	seen := echo.validatedEnv[0]
	for _, key := range []string{keyDBPassword, keySigningKey} {
		if strings.Contains(err.Error(), seen[key]) || strings.Contains(out2.String(), seen[key]) {
			t.Errorf("%s leaked through Docker's output", key)
		}
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Errorf("error does not show the redacted output: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "beta.env")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("beta.env was written although Compose rejected it")
	}
}

// echoingDocker fails validation with output that repeats every value it
// was given, as a verbose tool might.
type echoingDocker struct{ *fakeDocker }

func (e echoingDocker) ValidateCompose(ctx context.Context, file string, env map[string]string) (string, error) {
	_, err := e.fakeDocker.ValidateCompose(ctx, file, env)
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + env[k] + "\n")
	}
	return b.String(), err
}

func TestDryRunChangesNothing(t *testing.T) {
	dir := deployDir(t)
	if _, err := create(t, dir, "alpha", "alpha.flybirds.com", &fakeDocker{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, dir)
	var out bytes.Buffer
	docker := &fakeDocker{}
	deps := testDeps(docker, &out)
	deps.Random = failingReader{} // a dry run must not generate real secrets
	result, err := Create(context.Background(), Options{Code: "beta", Domain: "beta.flybirds.com", ACMEEmail: "it@flybirds.com", DryRun: true, DeployDir: dir}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Written {
		t.Error("a dry run reports a written file")
	}
	if after := snapshot(t, dir); !equalMaps(before, after) {
		t.Error("a dry run changed the deploy directory")
	}
	if strings.Join(docker.calls, ";") != "resources beta;validate beta" {
		t.Errorf("Docker calls = %v; want only the read-only check and validation", docker.calls)
	}
	for _, want := range []string{"Dry run", "beta.env", "beta_postgres_data", "beta_default", "beta-caddy-1", "nothing was written"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry-run output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestInterruptedProvisioningLeavesNoFile(t *testing.T) {
	dir := deployDir(t)
	if _, err := create(t, dir, "alpha", "alpha.flybirds.com", &fakeDocker{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, dir)
	deps := testDeps(&fakeDocker{}, &bytes.Buffer{})
	deps.beforeCommit = func() error { return errors.New("interrupted") }
	opts := Options{Code: "beta", Domain: "beta.flybirds.com", ACMEEmail: "it@flybirds.com", DeployDir: dir}
	if _, err := Create(context.Background(), opts, deps); err == nil {
		t.Fatal("an interrupted provisioning reported success")
	}
	if after := snapshot(t, dir); !equalMaps(before, after) {
		t.Errorf("an interrupted provisioning left files: %v", keys(snapshot(t, dir)))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Create(ctx, opts, testDeps(&fakeDocker{}, &bytes.Buffer{})); err == nil {
		t.Fatal("a cancelled provisioning reported success")
	}
	if after := snapshot(t, dir); !equalMaps(before, after) {
		t.Errorf("a cancelled provisioning left files: %v", keys(snapshot(t, dir)))
	}
	// A temporary file left by a killed process blocks that school until
	// someone has looked at it.
	if err := os.WriteFile(filepath.Join(dir, tempPrefix+"beta-123.env"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(context.Background(), opts, testDeps(&fakeDocker{}, &bytes.Buffer{})); err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("a stale temporary file was ignored: %v", err)
	}
	schools, _ := List(dir)
	if len(schools) != 1 || schools[0].Code != "alpha" {
		t.Errorf("List = %+v; temporary files must not count as schools", schools)
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestTemplateMustCoverEveryValue(t *testing.T) {
	dir := deployDir(t)
	path := filepath.Join(dir, templateFileName)
	data, _ := os.ReadFile(path)
	// A future required setting without a default must not be left empty.
	if err := os.WriteFile(path, append(data, []byte("\nSMS_API_KEY=\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := create(t, dir, "alpha", "alpha.flybirds.com", &fakeDocker{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "SMS_API_KEY") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "alpha.env")); !errors.Is(err, os.ErrNotExist) {
		t.Error("alpha.env was written from an incomplete template")
	}
}

func TestShellQuotingHandlesWindowsPaths(t *testing.T) {
	path := `C:\Users\Laptop Click's\Fly_Birds\deploy\alpha.env`
	if got, want := posixQuote(path), `'C:\Users\Laptop Click'\''s\Fly_Birds\deploy\alpha.env'`; got != want {
		t.Errorf("posixQuote = %s, want %s", got, want)
	}
	if got, want := powerShellQuote(path), `'C:\Users\Laptop Click''s\Fly_Birds\deploy\alpha.env'`; got != want {
		t.Errorf("powerShellQuote = %s, want %s", got, want)
	}
	if bash := exec.Command("bash", "-c", "printf %s "+posixQuote(path)); bash.Err == nil {
		if out, err := bash.Output(); err == nil && string(out) != path {
			t.Errorf("bash read %q back as %q", path, out)
		}
	}
	dir := deployDir(t) // contains a space and an apostrophe
	var out bytes.Buffer
	if _, err := create(t, dir, "alpha", "alpha.flybirds.com", &fakeDocker{}, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "bash:       docker compose --env-file "+posixQuote(filepath.Join(dir, "alpha.env"))) ||
		!strings.Contains(text, "PowerShell: docker compose --env-file "+powerShellQuote(filepath.Join(dir, "alpha.env"))) {
		t.Errorf("next steps do not quote the paths for both shells:\n%s", text)
	}
}

// TestComposeAcceptsProvisionedSchools runs the real "docker compose config"
// on two provisioned schools, one with the HTTPS edge and one without, in a
// directory whose path has spaces. It needs only the docker CLI, not a
// running container.
func TestComposeAcceptsProvisionedSchools(t *testing.T) {
	docker := FindDocker()
	if docker == nil {
		t.Skip("docker CLI not available")
	}
	if _, err := exec.Command("docker", "compose", "version").Output(); err != nil {
		t.Skip("docker compose not available")
	}
	dir := deployDir(t)
	deps := Deps{Docker: docker, PortFree: func(int) bool { return true }, Out: &bytes.Buffer{}}
	ctx := context.Background()
	// Real Docker is asked for existing resources only when a daemon runs;
	// otherwise the check is skipped and only "config" is used.
	skipCheck := exec.Command("docker", "info").Run() != nil
	alpha, err := Create(ctx, Options{Code: "m14test-alpha", Domain: "alpha.flybirds.com", ACMEEmail: "it@flybirds.com", DeployDir: dir, SkipDockerCheck: skipCheck}, deps)
	if err != nil {
		t.Fatal(err)
	}
	beta, err := Create(ctx, Options{Code: "m14test-beta", Domain: "beta.localhost", NoEdge: true, DeployDir: dir, SkipDockerCheck: skipCheck}, deps)
	if err != nil {
		t.Fatal(err)
	}
	type composeConfig struct {
		Name     string `json:"name"`
		Services map[string]struct {
			Environment map[string]*string `json:"environment"`
			Ports       []struct {
				HostIP    string `json:"host_ip"`
				Published string `json:"published"`
			} `json:"ports"`
			Volumes []struct {
				Source   string `json:"source"`
				Target   string `json:"target"`
				ReadOnly bool   `json:"read_only"`
			} `json:"volumes"`
		} `json:"services"`
		Volumes map[string]struct {
			Name string `json:"name"`
		} `json:"volumes"`
	}
	config := func(r *Result) composeConfig {
		cmd := exec.Command("docker", "compose", "--env-file", r.EnvFile, "-f", r.ComposeFile, "config", "--format", "json")
		cmd.Env = composeEnv(os.Environ(), nil)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("docker compose config for %s: %v: %s", r.Code, err, stderr.String())
		}
		var c composeConfig
		if err := json.Unmarshal(out, &c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	a, b := config(alpha), config(beta)
	if a.Name != "m14test-alpha" || b.Name != "m14test-beta" {
		t.Fatalf("project names %q and %q", a.Name, b.Name)
	}
	for key, volume := range a.Volumes {
		if volume.Name != "m14test-alpha_"+key {
			t.Errorf("alpha volume %s is named %s", key, volume.Name)
		}
	}
	for key, volume := range b.Volumes {
		if volume.Name != "m14test-beta_"+key {
			t.Errorf("beta volume %s is named %s", key, volume.Name)
		}
	}
	// The HTTPS edge from PR #22: Caddy reads the school's domain and
	// reaches web only through the read-only socket volume.
	caddy, ok := a.Services["caddy"]
	if !ok {
		t.Fatal("alpha has no caddy service with the edge profile")
	}
	if domain := caddy.Environment["SCHOOL_DOMAIN"]; domain == nil || *domain != "alpha.flybirds.com" {
		t.Errorf("caddy SCHOOL_DOMAIN = %v", domain)
	}
	socket := false
	for _, v := range caddy.Volumes {
		if v.Target == "/run/edge" {
			socket = v.Source == "edge_socket" && v.ReadOnly
		}
	}
	if !socket {
		t.Errorf("caddy does not mount edge_socket read-only: %+v", caddy.Volumes)
	}
	if _, ok := b.Services["caddy"]; ok {
		t.Error("beta without the edge profile has a caddy service")
	}
	for name, c := range map[string]composeConfig{"alpha": a, "beta": b} {
		web := c.Services["web"]
		if len(web.Ports) != 1 || web.Ports[0].HostIP != "127.0.0.1" {
			t.Errorf("%s web ports = %+v; want one port bound to 127.0.0.1", name, web.Ports)
		}
	}
	if a.Services["web"].Ports[0].Published != "8081" || b.Services["web"].Ports[0].Published != "8082" {
		t.Errorf("web ports %s and %s", a.Services["web"].Ports[0].Published, b.Services["web"].Ports[0].Published)
	}
}
