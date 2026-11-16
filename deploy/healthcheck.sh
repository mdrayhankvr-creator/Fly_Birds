#!/bin/sh
# One command to run after a deploy, or any time the school reports trouble.
# It checks the six things that actually break:
#
#   1. the containers are running and the backend reports itself healthy
#   2. the site answers over HTTPS with a valid certificate
#   3. the API answers
#   4. the database answers (/readyz)
#   5. the PDF renderer compiles a page in English and in Bangla
#   6. the disk has room, and so does the database volume
#
#   sudo install -m 0755 deploy/healthcheck.sh /usr/local/bin/flybirds-check
#   SCHOOL=greenvalley flybirds-check
#
# Settings (environment):
#   SCHOOL    the school's code, as in deploy/<code>.env   (required)
#   APP_DIR   the checkout                                 (default /opt/flybirds/app)
#   BASE_URL  what to check from outside   (default https://$SCHOOL_DOMAIN from the env file)
#   DISK_WARN percent full that counts as a failure        (default 85)
#
# Every check prints one line. The exit status is 0 only if all of them
# passed, so it can be used in a cron job or a monitor.

set -u

SCHOOL="${SCHOOL:-}"
APP_DIR="${APP_DIR:-/opt/flybirds/app}"
DISK_WARN="${DISK_WARN:-85}"

if [ -z "$SCHOOL" ]; then
    echo "set SCHOOL to the school's code (the name of deploy/<code>.env)" >&2
    exit 2
fi
ENV_FILE="$APP_DIR/deploy/$SCHOOL.env"
COMPOSE_FILE="$APP_DIR/deploy/compose.prod.yaml"
for path in "$ENV_FILE" "$COMPOSE_FILE"; do
    if [ ! -f "$path" ]; then
        echo "not found: $path" >&2
        exit 2
    fi
done

# Read SCHOOL_DOMAIN without running the file: it holds secrets, and nothing
# here needs them.
domain="$(sed -n 's/^SCHOOL_DOMAIN=//p' "$ENV_FILE" | tail -n 1 | tr -d '"'"'"' \r')"
BASE_URL="${BASE_URL:-https://$domain}"

compose() { docker compose -p "$SCHOOL" --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"; }

failures=0
pass() { echo "  OK    $1"; }
fail() { echo "  FAIL  $1"; failures=$((failures + 1)); }

echo "Fly_Birds check: $SCHOOL at $BASE_URL ($(date -u '+%Y-%m-%dT%H:%M:%SZ'))"

# 1. Containers. The backend's own health check is the database ping, so a
#    backend that is up but not healthy already tells us a lot.
state="$(compose ps --format '{{.Service}} {{.State}} {{.Health}}' 2>/dev/null)"
if [ -z "$state" ]; then
    fail "containers: none running for project $SCHOOL"
else
    echo "$state" | while read -r service rest; do
        echo "        $service: $rest"
    done
    for service in db backend web; do
        if ! echo "$state" | grep -q "^$service running"; then
            fail "containers: $service is not running"
        fi
    done
    # Older Compose versions print no health column; /readyz below is the real
    # verdict, so an unknown state is reported without failing the run.
    health="$(echo "$state" | sed -n 's/^backend running *//p')"
    case "$health" in
        healthy) pass "containers: backend is healthy" ;;
        '')      echo "        backend health: not reported by this Docker Compose" ;;
        *)       fail "containers: backend is $health, not healthy" ;;
    esac
fi

# 2. HTTPS from outside, certificate included. curl fails on an expired or
#    untrusted certificate, which is the usual cause of "the site is down".
if ! command -v curl >/dev/null 2>&1; then
    fail "https: curl is not installed on this host"
elif [ -z "$domain" ]; then
    fail "https: SCHOOL_DOMAIN is not set in $ENV_FILE"
else
    code="$(curl -fsS -o /dev/null -w '%{http_code}' --max-time 20 "$BASE_URL/" 2>/dev/null)"
    if [ "$code" = "200" ]; then
        pass "https: the site answers 200"
    else
        fail "https: the site answered '${code:-no response}' (certificate, DNS or ports 80/443?)"
    fi
    # Days left on the certificate, when openssl is available.
    if command -v openssl >/dev/null 2>&1; then
        until_date="$(echo | openssl s_client -servername "$domain" -connect "$domain:443" 2>/dev/null \
            | openssl x509 -noout -enddate 2>/dev/null | sed -n 's/^notAfter=//p')"
        if [ -n "$until_date" ]; then
            echo "        certificate valid until $until_date"
        fi
    fi
fi

# 3 and 4. The API and the database, as the application itself reports them.
for probe in healthz readyz; do
    code="$(curl -fsS -o /dev/null -w '%{http_code}' --max-time 20 "$BASE_URL/api/$probe" 2>/dev/null)"
    if [ "$code" = "200" ]; then
        case "$probe" in
            healthz) pass "api: /healthz answers 200" ;;
            readyz)  pass "database: /readyz answers 200" ;;
        esac
    else
        case "$probe" in
            healthz) fail "api: /healthz answered '${code:-no response}'" ;;
            readyz)  fail "database: /readyz answered '${code:-no response}' (PostgreSQL unreachable?)" ;;
        esac
    fi
done

# 5. The PDF renderer. Marksheets and admit cards are rendered by the typst
#    binary in the backend image with the bundled fonts and no network, so
#    compile a page with Latin and Bangla text exactly that way. This catches
#    a missing binary, a missing font and a read-only temporary directory
#    without needing anyone's password.
probe='#set page(width: 80mm, height: 40mm)
#set text(font: "Noto Sans", size: 10pt)
Marksheet renderer check
#text(font: "Noto Sans Bengali")[মার্কশিট পরীক্ষা]'
if compose exec -T backend sh -c '
    set -e
    work="$(mktemp -d)"
    trap "rm -rf \"$work\"" EXIT
    cat > "$work/probe.typ"
    typst compile --root "$work" --ignore-system-fonts --font-path /app/fonts "$work/probe.typ" "$work/probe.pdf" >/dev/null 2>&1
    head -c 5 "$work/probe.pdf" | grep -q "%PDF-"
' <<EOF >/dev/null 2>&1
$probe
EOF
then
    pass "pdf: the renderer made a page in English and Bangla"
else
    fail "pdf: the renderer could not make a page (typst binary, fonts or /tmp)"
fi

# 6. Disk. The archives, the images and the photos are what fill a small VM,
#    and PostgreSQL stops accepting writes when the disk is full. Check the
#    root filesystem and, when Docker keeps its data on another one, that too.
#    Fields are read from the end of the line, because a filesystem name can
#    contain spaces.
check_disk() {
    line="$(df -P "$1" 2>/dev/null | tail -n 1)"
    if [ -z "$line" ]; then
        fail "disk: cannot read the free space on $1"
        return
    fi
    used_pct="$(echo "$line" | awk '{print $(NF-1)}' | tr -d '%')"
    avail="$(echo "$line" | awk '{print $(NF-2)}')"
    case "$used_pct" in
        ''|*[!0-9]*) fail "disk: cannot read the usage of $1 from '$line'"; return ;;
    esac
    echo "        $1 is ${used_pct}% full, $((avail / 1024)) MiB free"
    if [ "$used_pct" -ge "$DISK_WARN" ]; then
        fail "disk: $1 is ${used_pct}% full (limit ${DISK_WARN}%); clear the Docker build cache or old archives"
    else
        pass "disk: $1 is ${used_pct}% full"
    fi
}
check_disk /
docker_root="$(docker info --format '{{.DockerRootDir}}' 2>/dev/null)"
if [ -n "$docker_root" ] && [ -d "$docker_root" ]; then
    root_device="$(df -P / | tail -n 1 | awk '{print $1}')"
    docker_device="$(df -P "$docker_root" | tail -n 1 | awk '{print $1}')"
    if [ "$docker_device" != "$root_device" ]; then
        check_disk "$docker_root"
    fi
fi
# What the archives and uploads are actually using, from inside the container.
usage="$(compose exec -T backend sh -c 'du -sh /app/backups /app/uploads 2>/dev/null' 2>/dev/null)"
if [ -n "$usage" ]; then
    echo "$usage" | while read -r size path; do echo "        $path: $size"; done
fi

echo
if [ "$failures" -eq 0 ]; then
    echo "All checks passed."
    exit 0
fi
echo "$failures check(s) failed."
exit 1
