#!/usr/bin/env bash
# Checks the production HTTPS path (Caddy -> nginx -> backend) of a running
# school stack that was started with the "edge" profile:
#
#   deploy/test-edge.sh deploy/<school>.env
#
# Each simulated visitor is its own container on the school's project network,
# so each has a distinct address. The checks send failed sign-ins for made-up
# accounts and exhaust the login rate limit for those test containers only.
# Use it on disposable or CI stacks, not on a school that is in use.
set -euo pipefail
# Git Bash on Windows would rewrite the container paths in docker arguments.
docker() { MSYS_NO_PATHCONV=1 command docker "$@"; }

env_file=${1:?usage: deploy/test-edge.sh <school env file>}
here=$(cd "$(dirname "$0")" && pwd)
setting() { sed -n "s/^$1=//p" "$env_file" | tail -1; }
project=$(setting COMPOSE_PROJECT_NAME)
domain=$(setting SCHOOL_DOMAIN)
web_port=$(setting WEB_PORT)
web_port=${web_port:-8081}
network="${project}_default"
curl_image=curlimages/curl:8.11.1
failures=0
clients=()

check() { # check <description> <condition result: 0 = pass> [detail]
  if [ "$2" -eq 0 ]; then echo "PASS $1${3:+ — $3}"; else echo "FAIL $1${3:+ — $3}"; failures=$((failures + 1)); fi
}
cleanup() { for c in "${clients[@]}"; do docker rm -f "$c" >/dev/null 2>&1 || true; done; }
trap cleanup EXIT

# start_client <name>: a container that stays up so its address stays the same.
start_client() {
  local name="${project}-edgetest-$1"
  docker rm -f "$name" >/dev/null 2>&1 || true
  docker run -d --rm --name "$name" --network "$network" --entrypoint sleep "$curl_image" 600 >/dev/null
  clients+=("$name")
}
address() { docker inspect -f "{{(index .NetworkSettings.Networks \"$network\").IPAddress}}" "${project}-edgetest-$1"; }
in_client() { local name=$1; shift; docker exec "${project}-edgetest-$name" sh -c "$*"; }

# Sign-in attempts through Caddy over HTTPS, one per made-up account so the
# API's per-account lockout never answers instead of the rate limit. Prints
# one HTTP status per attempt. Extra curl options (headers) follow the count.
edge_logins() { # edge_logins <client> <count> [curl options...]
  local client=$1 count=$2; shift 2
  in_client "$client" "for i in \$(seq $count); do curl -sk -o /dev/null -w '%{http_code}\n' --connect-to $domain:443:caddy:443 $* -H 'Content-Type: application/json' -d \"{\\\"email\\\":\\\"edge-test-\$i@example.invalid\\\",\\\"password\\\":\\\"not-the-password\\\"}\" https://$domain/api/auth/login; done"
}
count_429() { tr -d '\r' | grep -c '^429$' || true; }

echo "Edge checks for project '$project' at https://$domain"
start_client a; start_client b; start_client c; start_client d
echo "Visitors: A=$(address a) B=$(address b) C=$(address c) D=$(address d)"

# 1. HTTPS basics.
ready=$(in_client a "curl -sk --connect-to $domain:443:caddy:443 https://$domain/api/readyz")
check "HTTPS API readiness through Caddy" "$([ "$ready" = '{"data":{"status":"ready"}}' ]; echo $?)" "$ready"
redirect=$(in_client a "curl -s -o /dev/null -w '%{http_code} %{redirect_url}' --connect-to $domain:80:caddy:80 http://$domain/")
check "HTTP redirects permanently to HTTPS" "$([[ "$redirect" == 308\ https://$domain/* ]]; echo $?)" "$redirect"
headers=$(in_client a "curl -sk -D - -o /dev/null --connect-to $domain:443:caddy:443 https://$domain/")
check "HSTS header is set" "$(grep -qi '^strict-transport-security: max-age=31536000; includeSubDomains' <<<"$headers"; echo $?)"
check "Content-Security-Policy header is set" "$(grep -qi "^content-security-policy: default-src 'self'; script-src 'self'" <<<"$headers"; echo $?)"
check "Content-Security-Policy is sent once" "$([ "$(grep -ci '^content-security-policy:' <<<"$headers")" -eq 1 ]; echo $?)"

# 2. Visitor A reaches the limit; visitor B is unaffected.
a_codes=$(edge_logins a 40)
check "visitor A is rate-limited after a burst of sign-ins" "$([ "$(count_429 <<<"$a_codes")" -ge 1 ]; echo $?)" "$(count_429 <<<"$a_codes") of 40 answered 429"
b_codes=$(edge_logins b 5)
check "visitor B can still sign in while A is limited" "$([ "$(count_429 <<<"$b_codes")" -eq 0 ]; echo $?)" "$(tr '\n' ' ' <<<"$b_codes")"

# 3. Forged forwarding headers through Caddy neither lift A's limit nor
#    push the limit onto someone else. The limit refills one request every
#    two seconds, so a few of A's requests may pass; with a working forgery
#    all ten would.
forged=$(edge_logins a 10 -H "'X-Forwarded-For: 198.51.100.7'" -H "'X-Real-IP: 198.51.100.7'" -H "'Forwarded: for=198.51.100.7'")
check "visitor A cannot escape the limit with forged forwarding headers" "$([ "$(count_429 <<<"$forged")" -ge 6 ]; echo $?)" "$(count_429 <<<"$forged") of 10 answered 429"
framed=$(edge_logins c 5 -H "'X-Forwarded-For: $(address a)'")
check "visitor C claiming to be A is not limited as A" "$([ "$(count_429 <<<"$framed")" -eq 0 ]; echo $?)" "$(tr '\n' ' ' <<<"$framed")"

# 4. Direct connections to the web container (bypassing Caddy) are limited by
#    their own address: a fresh forged address on every request does not help,
#    and B's allowance is untouched.
direct=$(in_client d "for i in \$(seq 40); do curl -s -o /dev/null -w '%{http_code}\n' -H \"X-Forwarded-For: 10.77.\$i.1\" -H \"X-Real-IP: 10.77.\$i.1\" -H 'Content-Type: application/json' -d \"{\\\"email\\\":\\\"edge-direct-\$i@example.invalid\\\",\\\"password\\\":\\\"not-the-password\\\"}\" http://web/api/auth/login; done")
check "direct access with forged addresses is still rate-limited" "$([ "$(count_429 <<<"$direct")" -ge 1 ]; echo $?)" "$(count_429 <<<"$direct") of 40 answered 429"
b_after=$(edge_logins b 5)
check "visitor B is unaffected by D's forged direct requests" "$([ "$(count_429 <<<"$b_after")" -eq 0 ]; echo $?)" "$(tr '\n' ' ' <<<"$b_after")"

#    The same holds on the host port. This uses up the limit for this host's
#    address for about a minute, so run browser tests before this script.
host_direct=$(for i in $(seq 40); do curl -s -o /dev/null -w '%{http_code}\n' -H "X-Forwarded-For: 10.78.$i.1" -H "X-Real-IP: 10.78.$i.1" -H 'Content-Type: application/json' -d "{\"email\":\"edge-host-$i@example.invalid\",\"password\":\"not-the-password\"}" "http://127.0.0.1:$web_port/api/auth/login"; done)
check "direct access on the host port with forged addresses is still rate-limited" "$([ "$(count_429 <<<"$host_direct")" -ge 1 ]; echo $?)" "$(count_429 <<<"$host_direct") of 40 answered 429"

# 5. nginx records the real visitor addresses for requests through Caddy, and
#    never an address a client made up.
logs=$(docker logs --since 5m "${project}-web-1" 2>/dev/null || true)
check "nginx sees visitor A's address behind Caddy" "$(grep -q "^$(address a) " <<<"$logs"; echo $?)"
check "nginx sees visitor B's address behind Caddy" "$(grep -q "^$(address b) " <<<"$logs"; echo $?)"
check "nginx never records a forged client address" "$(! grep -qE '^(198\.51\.100\.7|10\.77\.|10\.78\.)' <<<"$logs"; echo $?)"

echo
if [ "$failures" -eq 0 ]; then echo "All edge checks passed"; else echo "$failures edge check(s) failed"; exit 1; fi
