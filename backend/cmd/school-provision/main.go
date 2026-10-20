// Command school-provision prepares a new school's isolated deployment
// configuration (see docs/DEPLOYMENT.md). It writes one Git-ignored
// environment file per school and never starts, changes or removes Docker
// resources.
//
//	go run ./cmd/school-provision create --code greenvalley --domain greenvalley.flybirds.com --acme-email it@school.example.com.bd
//	go run ./cmd/school-provision create --code greenvalley --domain greenvalley.flybirds.com --acme-email ... --dry-run
//	go run ./cmd/school-provision list
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/provision"
)

const usage = `Usage:
  school-provision create --code CODE --domain DOMAIN [--acme-email EMAIL]
                          [--web-port PORT] [--no-edge] [--dry-run]
                          [--skip-docker-check] [--deploy-dir DIR]
  school-provision list [--deploy-dir DIR]

create  validates the school and writes DIR/CODE.env from DIR/school.env.example
        with a new database password and signing key. It never overwrites a
        file and never starts containers. --dry-run only prints the plan.
list    shows the provisioned schools (no secrets).

CODE    3-32 lower-case letters, digits and single hyphens, starting with a letter.
DOMAIN  the school's host name, for example greenvalley.flybirds.com
        (localhost or *.localhost for a local trial).
DIR     defaults to the deploy directory of the repository containing the
        current directory.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "create":
		return create(ctx, args[1:], stdout, stderr)
	case "list":
		return list(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func create(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var opts provision.Options
	flags.StringVar(&opts.Code, "code", "", "school code (Compose project name)")
	flags.StringVar(&opts.Domain, "domain", "", "school host name")
	flags.StringVar(&opts.ACMEEmail, "acme-email", "", "certificate contact address (not needed for localhost names)")
	flags.IntVar(&opts.WebPort, "web-port", 0, "local web port (default: first free from 8081)")
	flags.BoolVar(&opts.NoEdge, "no-edge", false, "do not run the school's own Caddy TLS edge")
	flags.BoolVar(&opts.DryRun, "dry-run", false, "validate and print the plan without writing anything")
	flags.BoolVar(&opts.SkipDockerCheck, "skip-docker-check", false, "do not ask Docker about existing resources")
	flags.StringVar(&opts.DeployDir, "deploy-dir", "", "directory with compose.prod.yaml and school.env.example")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument %q\n", flags.Arg(0))
		return 2
	}
	if opts.Code == "" || opts.Domain == "" {
		fmt.Fprint(stderr, "create needs --code and --domain\n\n"+usage)
		return 2
	}
	dir, err := deployDir(opts.DeployDir)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	opts.DeployDir = dir
	if _, err := provision.Create(ctx, opts, provision.Deps{Docker: provision.FindDocker(), Out: stdout}); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

func list(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dirFlag := flags.String("deploy-dir", "", "directory with the school environment files")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	dir, err := deployDir(*dirFlag)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	schools, err := provision.List(dir)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if len(schools) == 0 {
		fmt.Fprintln(stdout, "No schools are provisioned in", dir)
		return 0
	}
	fmt.Fprintf(stdout, "%-20s %-36s %-6s %-5s %s\n", "CODE", "DOMAIN", "PORT", "EDGE", "FILE")
	for _, s := range schools {
		edge := "no"
		if s.Profiles != "" {
			edge = "yes"
		}
		line := fmt.Sprintf("%-20s %-36s %-6d %-5s %s", s.Code, s.Domain, s.WebPort, edge, filepath.Base(s.File))
		if s.Problem != "" {
			line += "  (problem: " + s.Problem + ")"
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}

// deployDir returns the given directory, or finds deploy/ above the current
// directory, so the command works from the repository root or backend/.
func deployDir(given string) (string, error) {
	if given != "" {
		return filepath.Abs(given)
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "deploy")
		if _, err := os.Stat(filepath.Join(candidate, "compose.prod.yaml")); err == nil {
			return candidate, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no deploy/compose.prod.yaml above the current directory; pass --deploy-dir")
		}
		dir = parent
	}
}
