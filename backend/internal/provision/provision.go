package provision

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	composeFileName  = "compose.prod.yaml"
	templateFileName = "school.env.example"
	// tempPrefix marks an environment file still being written. It ends in
	// ".env", so Git ignores it like a finished school file.
	tempPrefix = ".provision-"
)

// Options describe one school to provision.
type Options struct {
	Code      string
	Domain    string
	ACMEEmail string
	// WebPort is the school's local web port; 0 picks the first free one
	// from FirstWebPort.
	WebPort int
	// NoEdge leaves the per-school Caddy TLS edge off, for a shared reverse
	// proxy or a plain local trial.
	NoEdge          bool
	DryRun          bool
	SkipDockerCheck bool
	// DeployDir holds compose.prod.yaml, the template and the school files.
	DeployDir string
}

// Deps are the provisioner's side effects, replaceable in tests.
type Deps struct {
	Random io.Reader
	// Docker is nil when the docker CLI is not available.
	Docker   Docker
	PortFree func(port int) bool
	Now      func() time.Time
	Out      io.Writer
	// beforeCommit runs after the temporary file is complete and before it
	// is linked into place; tests use it to simulate an interruption.
	beforeCommit func() error
}

// Result describes what Create did or, in a dry run, would do.
type Result struct {
	Code, Domain, EnvFile, ComposeFile string
	WebPort                            int
	Edge                               bool
	Volumes, Containers                []string
	Network                            string
	Written                            bool
}

// School is one provisioned school, as listed from its environment file.
type School struct {
	File, Code, Domain, Profiles string
	WebPort                      int
	Problem                      string
}

// Create validates a new school and writes its environment file, or only
// prints the plan when opts.DryRun is set. It never overwrites, deletes or
// starts anything.
func Create(ctx context.Context, opts Options, deps Deps) (*Result, error) {
	if deps.Random == nil {
		deps.Random = defaultRandom
	}
	if deps.PortFree == nil {
		deps.PortFree = portFree
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Out == nil {
		deps.Out = io.Discard
	}
	out := deps.Out

	if err := ValidateCode(opts.Code); err != nil {
		return nil, err
	}
	domain, err := NormalizeDomain(opts.Domain)
	if err != nil {
		return nil, err
	}
	email := opts.ACMEEmail
	switch {
	case email == "" && IsLocalDomain(domain):
		// Unused: Caddy issues local certificates for these names itself.
		email = "operator@localhost.localdomain"
	case email == "":
		return nil, fmt.Errorf("--acme-email is required for %s: Let's Encrypt sends certificate notices there", domain)
	default:
		if err := ValidateACMEEmail(email); err != nil {
			return nil, err
		}
	}

	dir, err := filepath.Abs(opts.DeployDir)
	if err != nil {
		return nil, err
	}
	composeFile := filepath.Join(dir, composeFileName)
	templateData, err := os.ReadFile(filepath.Join(dir, templateFileName))
	if err != nil {
		return nil, fmt.Errorf("read the environment template: %w", err)
	}
	template, err := parseEnv(string(templateData))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", templateFileName, err)
	}
	services, volumes, err := composeSections(composeFile)
	if err != nil {
		return nil, fmt.Errorf("read the production Compose file: %w", err)
	}

	envFile := filepath.Join(dir, opts.Code+".env")
	if _, err := os.Lstat(envFile); err == nil {
		return nil, fmt.Errorf("school %q is already provisioned (%s); the provisioner never overwrites a school's file", opts.Code, envFile)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if stale, _ := filepath.Glob(filepath.Join(dir, tempPrefix+opts.Code+"-*.env")); len(stale) > 0 {
		return nil, fmt.Errorf("an earlier provisioning of %q was interrupted and left %s; it may contain secrets: check and delete it, then try again", opts.Code, stale[0])
	}
	schools, err := List(dir)
	if err != nil {
		return nil, err
	}
	usedPorts := map[int]string{}
	var edgeSchools []string
	for _, school := range schools {
		switch {
		// Files written by hand may differ in case; names are case-insensitive.
		case strings.EqualFold(school.Code, opts.Code):
			return nil, fmt.Errorf("%s already uses the Compose project name %q", school.File, opts.Code)
		case strings.EqualFold(school.Domain, domain):
			return nil, fmt.Errorf("%s already uses the domain %s", school.File, domain)
		}
		if school.WebPort > 0 {
			usedPorts[school.WebPort] = school.File
		}
		if strings.Contains(school.Profiles, edgeProfile) {
			edgeSchools = append(edgeSchools, school.Code)
		}
	}

	port := opts.WebPort
	if port == 0 {
		for port = FirstWebPort; port <= maxWebPort && (usedPorts[port] != "" || !deps.PortFree(port)); port++ {
		}
		if port > maxWebPort {
			return nil, fmt.Errorf("no free web port from %d", FirstWebPort)
		}
	} else {
		if err := ValidateWebPort(port); err != nil {
			return nil, err
		}
		if file := usedPorts[port]; file != "" {
			return nil, fmt.Errorf("web port %d is already assigned to %s", port, file)
		}
		if !deps.PortFree(port) {
			return nil, fmt.Errorf("web port %d is in use on this machine; choose another with --web-port or omit it to pick a free one", port)
		}
	}

	var warnings []string
	switch {
	case opts.SkipDockerCheck:
		warnings = append(warnings, "Docker check skipped: existing containers or volumes of project "+opts.Code+" were not looked for.")
	case deps.Docker == nil:
		warnings = append(warnings, "The docker command was not found: existing containers or volumes of project "+opts.Code+" were not looked for, and the Compose file was not validated. Run `docker volume ls` before starting the school.")
	default:
		existing, err := deps.Docker.ProjectResources(ctx, opts.Code)
		if err != nil {
			return nil, fmt.Errorf("could not check Docker for an existing project %q (%v); start Docker, or pass --skip-docker-check if you have checked by hand", opts.Code, err)
		}
		if len(existing) > 0 {
			return nil, fmt.Errorf("Docker already has resources for project %q: %s; choose another code (existing school data is never reused or removed)", opts.Code, strings.Join(existing, ", "))
		}
	}
	edge := !opts.NoEdge
	if edge && len(edgeSchools) > 0 {
		warnings = append(warnings, fmt.Sprintf("%s already use(s) the edge profile. Only one school per server can bind ports 80 and 443; on one server use --no-edge and a shared reverse proxy (docs/DEPLOYMENT.md, section 5).", strings.Join(edgeSchools, ", ")))
	}

	// A dry run validates with throwaway values, so it never holds the
	// secrets the real run will generate.
	random := deps.Random
	if opts.DryRun {
		random = zeroReader{}
	}
	sec, err := newSecrets(random)
	if err != nil {
		return nil, err
	}
	profiles := ""
	if edge {
		profiles = edgeProfile
	}
	set := map[string]string{
		keyProject: opts.Code, keyProfiles: profiles, keyDomain: domain, keyACMEEmail: email,
		keyDBPassword: sec.dbPassword, keySigningKey: sec.signingKey, keyWebPort: strconv.Itoa(port),
	}
	content, err := renderEnv(template, set, []string{
		fmt.Sprintf("Fly_Birds school %q (%s), generated by school-provision on %s.", opts.Code, domain, deps.Now().UTC().Format("2006-01-02")),
		"Contains this school's secrets: keep it private, never commit or share it, and",
		"keep a copy with the school's off-site backups (docs/OPERATIONS.md).",
	})
	if err != nil {
		return nil, err
	}
	if deps.Docker != nil && !opts.SkipDockerCheck {
		values := envValues(mustParse(content))
		if output, err := deps.Docker.ValidateCompose(ctx, composeFile, values); err != nil {
			return nil, fmt.Errorf("docker compose rejected the configuration: %s", strings.TrimSpace(redact(output, sec.dbPassword, sec.signingKey)))
		}
	}

	result := &Result{Code: opts.Code, Domain: domain, EnvFile: envFile, ComposeFile: composeFile, WebPort: port, Edge: edge, Network: opts.Code + "_default"}
	for _, volume := range volumes {
		// Only Caddy uses these; web always mounts edge_socket.
		if !edge && (volume == "caddy_data" || volume == "caddy_config") {
			continue
		}
		result.Volumes = append(result.Volumes, opts.Code+"_"+volume)
	}
	for _, service := range services {
		if service == "caddy" && !edge {
			continue
		}
		result.Containers = append(result.Containers, opts.Code+"-"+service+"-1")
	}
	printPlan(out, result, opts.DryRun, warnings)
	if opts.DryRun {
		fmt.Fprintln(out, "\nDry run: nothing was written and no container was started.")
		return result, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("stopped before writing: %w", err)
	}
	if err := writeExclusive(dir, opts.Code, envFile, content, deps.beforeCommit); err != nil {
		return nil, err
	}
	result.Written = true
	printNextSteps(out, result)
	return result, nil
}

// writeExclusive writes content to a private temporary file in dir, flushes
// it, and hard-links it to target. Linking fails if target exists, so an
// existing file is never replaced, and target only ever appears complete.
// The temporary file is always removed.
func writeExclusive(dir, code, target, content string, beforeCommit func() error) error {
	temp, err := os.CreateTemp(dir, tempPrefix+code+"-*.env")
	if err != nil {
		return fmt.Errorf("create the environment file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	// Owner-only on Unix. On Windows the file inherits the folder's access
	// list, normally the operator's own profile.
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("restrict the environment file: %w", err)
	}
	if _, err := temp.WriteString(content); err != nil {
		temp.Close()
		return fmt.Errorf("write the environment file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("flush the environment file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close the environment file: %w", err)
	}
	if beforeCommit != nil {
		if err := beforeCommit(); err != nil {
			return fmt.Errorf("stopped before the school file was created: %w", err)
		}
	}
	if err := os.Link(tempName, target); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s appeared while provisioning; it was not changed", target)
		}
		return fmt.Errorf("create %s: %w", target, err)
	}
	return nil
}

// List reads every school environment file in dir. It never returns
// secrets.
func List(dir string) ([]School, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.env"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var schools []School
	for _, path := range paths {
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") {
			continue
		}
		school := School{File: path}
		data, err := os.ReadFile(path)
		if err != nil {
			school.Problem = err.Error()
			schools = append(schools, school)
			continue
		}
		lines, err := parseEnv(string(data))
		if err != nil {
			school.Problem = err.Error()
			schools = append(schools, school)
			continue
		}
		values := envValues(lines)
		school.Code, school.Domain, school.Profiles = values[keyProject], values[keyDomain], values[keyProfiles]
		school.WebPort, _ = strconv.Atoi(values[keyWebPort])
		var missing []string
		for _, key := range []string{keyProject, keyDomain, keyDBPassword, keySigningKey} {
			if strings.TrimSpace(values[key]) == "" {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			school.Problem = "no value for " + strings.Join(missing, ", ")
		}
		schools = append(schools, school)
	}
	return schools, nil
}

func mustParse(content string) []envLine {
	lines, err := parseEnv(content)
	if err != nil {
		panic(err) // content was rendered by renderEnv
	}
	return lines
}

func portFree(port int) bool {
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
