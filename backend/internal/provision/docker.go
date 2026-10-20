package provision

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Docker is the provisioner's only view of Docker. Both methods are
// read-only: the provisioner never creates, starts or removes anything.
type Docker interface {
	// ProjectResources lists the containers, volumes and networks that
	// already belong to a Compose project.
	ProjectResources(ctx context.Context, project string) ([]string, error)
	// ValidateCompose runs "docker compose config" for the given file with
	// the given variables and returns its combined output.
	ValidateCompose(ctx context.Context, composeFile string, env map[string]string) (string, error)
}

// DockerCLI uses the docker command line.
type DockerCLI struct{ Path string }

// FindDocker returns the docker CLI, or nil when it is not installed.
func FindDocker() Docker {
	path, err := exec.LookPath("docker")
	if err != nil {
		return nil
	}
	return DockerCLI{Path: path}
}

func (d DockerCLI) ProjectResources(ctx context.Context, project string) ([]string, error) {
	filter := "label=com.docker.compose.project=" + project
	var found []string
	for _, args := range [][]string{
		{"ps", "--all", "--filter", filter, "--format", "container {{.Names}}"},
		{"volume", "ls", "--filter", filter, "--format", "volume {{.Name}}"},
		{"network", "ls", "--filter", filter, "--format", "network {{.Name}}"},
	} {
		cmd := exec.CommandContext(ctx, d.Path, args...)
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("docker %s: %s", args[0], strings.TrimSpace(errOut.String()))
		}
		for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
			if line != "" {
				found = append(found, line)
			}
		}
	}
	sort.Strings(found)
	return found, nil
}

func (d DockerCLI) ValidateCompose(ctx context.Context, composeFile string, env map[string]string) (string, error) {
	cmd := exec.CommandContext(ctx, d.Path, "compose", "-f", composeFile, "config", "--quiet")
	cmd.Env = composeEnv(os.Environ(), env)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// composeEnv passes the school's values as variables and drops any COMPOSE_*
// settings or template keys from the operator's shell, which would otherwise
// change what Compose validates.
func composeEnv(base []string, set map[string]string) []string {
	var env []string
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, overridden := set[key]; overridden || strings.HasPrefix(strings.ToUpper(key), "COMPOSE_") {
			continue
		}
		env = append(env, entry)
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+set[key])
	}
	return env
}

// redact replaces every secret in text, so output from other tools can be
// shown safely.
func redact(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	return text
}
