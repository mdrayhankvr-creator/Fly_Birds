#!/bin/sh
# Nightly backup for one school, run by cron on the server.
#
# It makes one archive (the whole database and every uploaded file), copies it
# out of the Docker volume onto the host disk, checks the copy against the
# archive's SHA-256, and keeps only the newest KEEP copies on the host. The
# application prunes its own copies inside the volume to BACKUP_RETENTION.
#
# Keeping a copy on the host matters: an archive that exists only inside the
# project's volume is lost with the volume. Copy the host directory somewhere
# else as well (section 8 of docs/HOSTING_FREE.md) — a backup on the same
# machine as the data is not yet a backup.
#
#   sudo install -m 0755 deploy/backup-daily.sh /usr/local/bin/flybirds-backup
#   sudo crontab -e
#   15 2 * * *  SCHOOL=greenvalley /usr/local/bin/flybirds-backup >> /var/log/flybirds-backup.log 2>&1
#
# Settings (environment, all optional except SCHOOL):
#   SCHOOL   the school's code, as in deploy/<code>.env   (required)
#   APP_DIR  the checkout                                 (default /opt/flybirds/app)
#   OUT_DIR  where host copies are kept                   (default /opt/flybirds/backups/<school>)
#   KEEP     how many host copies to keep                 (default 7)
#
# Exit status is 0 only when a verified copy is on the host, so cron mails you
# on any failure.

set -eu

SCHOOL="${SCHOOL:-}"
APP_DIR="${APP_DIR:-/opt/flybirds/app}"
KEEP="${KEEP:-7}"

if [ -z "$SCHOOL" ]; then
    echo "set SCHOOL to the school's code (the name of deploy/<code>.env)" >&2
    exit 2
fi
OUT_DIR="${OUT_DIR:-/opt/flybirds/backups/$SCHOOL}"
ENV_FILE="$APP_DIR/deploy/$SCHOOL.env"
COMPOSE_FILE="$APP_DIR/deploy/compose.prod.yaml"

for path in "$ENV_FILE" "$COMPOSE_FILE"; do
    if [ ! -f "$path" ]; then
        echo "not found: $path" >&2
        exit 2
    fi
done
case "$KEEP" in
    ''|*[!0-9]*) echo "KEEP must be a whole number" >&2; exit 2 ;;
esac
if [ "$KEEP" -lt 1 ]; then
    echo "KEEP must be at least 1" >&2
    exit 2
fi

log() { echo "$(date -u '+%Y-%m-%dT%H:%M:%SZ') $SCHOOL $*"; }
compose() { docker compose -p "$SCHOOL" --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"; }

mkdir -p "$OUT_DIR"
chmod 700 "$OUT_DIR"

# 1. Make the archive. create-backup prints "<name> <bytes> <sha256>" and
#    exits non-zero if nothing was written, so set -e stops us here on failure.
log "starting"
line="$(compose exec -T backend /app/create-backup)"
name="$(echo "$line" | awk '{print $1}')"
bytes="$(echo "$line" | awk '{print $2}')"
sum="$(echo "$line" | awk '{print $3}')"
if [ -z "$name" ] || [ -z "$sum" ]; then
    log "create-backup printed nothing usable: $line"
    exit 1
fi
log "archive $name ($bytes bytes) made in the volume"

# 2. Copy it onto the host. cat through exec works whether or not the
#    container is running as root and needs no volume path on the host.
tmp="$OUT_DIR/.$name.part"
trap 'rm -f "$tmp"' EXIT INT TERM
compose exec -T backend cat "/app/backups/$name" > "$tmp"

# 3. Check the copy. A truncated or corrupted copy is worse than none,
#    because it looks like a backup.
if command -v sha256sum >/dev/null 2>&1; then
    copied="$(sha256sum "$tmp" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
    copied="$(shasum -a 256 "$tmp" | awk '{print $1}')"
else
    echo "no sha256sum or shasum on this host; cannot verify the copy" >&2
    exit 1
fi
if [ "$copied" != "$sum" ]; then
    log "the copy does not match the archive ($copied != $sum)"
    exit 1
fi
# tar reads the whole archive, so this also catches a broken gzip stream. It
# is fed on standard input because a path with a colon in it (a Windows drive
# letter, say) is otherwise read as host:path.
if ! tar -tzf - < "$tmp" >/dev/null 2>&1; then
    log "the copy is not a readable tar.gz"
    exit 1
fi
mv "$tmp" "$OUT_DIR/$name"
chmod 600 "$OUT_DIR/$name"
trap - EXIT INT TERM
log "copied and verified $OUT_DIR/$name"

# 4. Keep the newest KEEP copies. The names sort by date, so the list is
#    newest first and everything after KEEP goes.
cd "$OUT_DIR"
removed=0
for old in $(ls -1 flybirds-backup-*.tar.gz 2>/dev/null | sort -r | tail -n +"$((KEEP + 1))"); do
    rm -f -- "$old"
    log "removed old copy $old"
    removed=$((removed + 1))
done
kept="$(ls -1 flybirds-backup-*.tar.gz 2>/dev/null | wc -l | tr -d ' ')"
log "done: $kept copy/copies on the host, $removed removed"
