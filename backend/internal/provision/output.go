package provision

import (
	"fmt"
	"io"
	"strings"
)

func printPlan(out io.Writer, r *Result, dryRun bool, warnings []string) {
	if dryRun {
		fmt.Fprintln(out, "Dry run: planned provisioning")
	} else {
		fmt.Fprintln(out, "Provisioning")
	}
	row := func(label, value string) { fmt.Fprintf(out, "  %-18s %s\n", label, value) }
	row("School code", r.Code+" (Compose project name)")
	row("Domain", r.Domain)
	row("Environment file", r.EnvFile+" (Git-ignored)")
	if r.Edge {
		row("TLS edge", "on: Caddy serves https://"+r.Domain+" on ports 80 and 443")
	} else {
		row("TLS edge", "off: serve it through a shared reverse proxy, or locally over HTTP")
	}
	row("Local web port", fmt.Sprintf("127.0.0.1:%d", r.WebPort))
	saved := "generated and saved to the file, not shown"
	if dryRun {
		saved = "would be generated and saved to the file; never shown"
	}
	row("Database password", "48 random hex characters, "+saved)
	row("Signing key", "32 random bytes (base64), "+saved)
	fmt.Fprintln(out, "  Docker resources the school gets on its first start (none are created now):")
	row("  Volumes", strings.Join(r.Volumes, ", "))
	row("  Network", r.Network)
	row("  Containers", strings.Join(r.Containers, ", "))
	for _, warning := range warnings {
		fmt.Fprintln(out, "  Warning: "+warning)
	}
}

func printNextSteps(out io.Writer, r *Result) {
	fmt.Fprintf(out, "\nCreated %s. Nothing has been started.\n\nNext steps:\n", r.EnvFile)
	compose := func(quote func(string) string, args string) string {
		return "docker compose --env-file " + quote(r.EnvFile) + " -f " + quote(r.ComposeFile) + " " + args
	}
	command := func(args string) {
		posix, ps := compose(posixQuote, args), compose(powerShellQuote, args)
		if posix == ps {
			fmt.Fprintln(out, "       "+posix)
			return
		}
		fmt.Fprintln(out, "       bash:       "+posix)
		fmt.Fprintln(out, "       PowerShell: "+ps)
	}
	fmt.Fprintln(out, "  1. Start the school (builds the images on first use):")
	command("up -d --build --wait")
	fmt.Fprintln(out, "  2. Create its first administrator; the command asks for the password and")
	fmt.Fprintln(out, "     needs an interactive terminal (in Git Bash prefix it with MSYS_NO_PATHCONV=1):")
	command("exec backend /app/create-user -email <administrator email> -role admin")
	if r.Edge {
		fmt.Fprintf(out, "  3. Sign in at https://%s/login", r.Domain)
		if IsLocalDomain(r.Domain) {
			fmt.Fprint(out, " (Caddy's local certificate: expect a browser warning)")
		}
		fmt.Fprintln(out)
	} else {
		fmt.Fprintf(out, "  3. Sign in at http://localhost:%d/login on this machine, or through your HTTPS reverse proxy.\n", r.WebPort)
	}
	fmt.Fprintln(out, "  4. Set the school's name, logo and colours in Admin > School settings.")
}

// posixQuote quotes a word for bash and other POSIX shells.
func posixQuote(word string) string {
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// powerShellQuote quotes a word for PowerShell.
func powerShellQuote(word string) string {
	return "'" + strings.ReplaceAll(word, "'", "''") + "'"
}
