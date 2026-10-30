#!/bin/sh
# Real end-to-end check of adding a server, with nothing mocked: the deployable
# image running with the production restrictions, a Debian 13 server with sshd,
# and Chromium going through the UI. The install script the UI generates is run
# on that server, then the panel must monitor it and run a command there.
set -u
IMAGE=$1
ENGINE=${ENGINE:-podman}
NET=panel-e2e
HERE=$(cd "$(dirname "$0")" && pwd)
W=$(mktemp -d)
mkdir -p "$HERE/shots" # ignored by git: a fresh clone does not have it
FAILED=0
result() { if [ "$1" = 0 ]; then echo "PASS $2"; else echo "FAIL $2"; FAILED=1; fi; }
cleanup() { $ENGINE rm -f e2e-panel e2e-server e2e-browser >/dev/null 2>&1 || true; $ENGINE network rm $NET >/dev/null 2>&1 || true; }
trap 'cleanup; rm -rf "$W"' EXIT
cleanup
wait_for() { # seconds, description, command
	n=0
	until eval "$3"; do
		n=$((n + 2)); [ $n -lt "$1" ] || { echo "FAIL timed out waiting for $2"; exit 1; }
		sleep 2
	done
}

$ENGINE network create $NET >/dev/null
$ENGINE run -d --name e2e-server --network $NET docker.io/library/debian:trixie sh -c \
	'apt-get update -qq && apt-get install -y -qq openssh-server curl ca-certificates >/dev/null && mkdir -p /run/sshd && ssh-keygen -A && rm -f /etc/ssh/ssh_host_rsa_key* && touch /ready && exec /usr/sbin/sshd -D -e' >/dev/null
$ENGINE run -d --name e2e-panel --network $NET --user 10001:10001 --read-only --cap-drop ALL --security-opt no-new-privileges \
	--tmpfs /tmp:size=8m,mode=1777 --tmpfs /var/lib/serverwarden:rw,size=64m,mode=0700,U "$IMAGE" >/dev/null

wait_for 300 "the Debian server" "$ENGINE exec e2e-server test -f /ready"
wait_for 60 "the setup link" "$ENGINE logs e2e-panel 2>&1 | grep -q 'setup='"
CODE=$($ENGINE logs e2e-panel 2>&1 | grep -o 'setup=[A-Za-z0-9_-]*' | head -1 | cut -d= -f2)
FP=$($ENGINE exec e2e-server ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub | cut -d' ' -f2)
echo "server fingerprint: $FP"

$ENGINE run -d --name e2e-browser --network $NET -e CODE="$CODE" -e FP="$FP" \
	-v "$HERE":/test:Z -v "$HERE/shots":/shots:Z -v "$W":/out:Z docker.io/library/alpine:3.22 sh /test/ui-check.sh install-e2e.mjs >/dev/null
wait_for 600 "the install script from the UI" "[ -s '$W/install.sh' ] || ! $ENGINE container exists e2e-browser || [ \"\$($ENGINE inspect -f '{{.State.Running}}' e2e-browser)\" != true ]"

if [ -s "$W/install.sh" ]; then
	$ENGINE cp "$W/install.sh" e2e-server:/root/install.sh
	$ENGINE exec e2e-server sh /root/install.sh > "$W/install.log" 2>&1; result $? "install script runs on Debian 13 (exit status 0)"
	sed 's/^/  | /' "$W/install.log"
	grep -q "$FP" "$W/install.log"; result $? "script prints the server fingerprint"
	$ENGINE exec e2e-server sh -c 'grep -q "\"panel_commands\": true" /etc/serverwarden/agent.json'; result $? "agent.json accepts panel commands"
	[ "$($ENGINE exec e2e-server grep -c '^restrict,command="/usr/local/bin/serverwarden-agent' /root/.ssh/authorized_keys)" = 2 ]; result $? "two restricted keys in authorized_keys"
	$ENGINE exec e2e-server sh /root/install.sh > "$W/again.log" 2>&1 && [ "$($ENGINE exec e2e-server grep -c 'serverwarden-' /root/.ssh/authorized_keys)" = 2 ]; result $? "running it again changes nothing"
	touch "$W/installed"
else
	result 1 "the UI produced an install script"
fi

STATUS=$($ENGINE wait e2e-browser)
$ENGINE logs e2e-browser 2>&1 | grep -v '^npm'

# Secrets at rest in the panel's data directory.
D=/var/lib/serverwarden
[ -s "$W/totp-secret" ] && $ENGINE exec e2e-panel test -s $D/totp.json && ! $ENGINE exec e2e-panel grep -q "$(cat "$W/totp-secret")" $D/totp.json; result $? "totp.json does not hold the TOTP secret"
# Every check below fails when the file is missing: absence must never pass.
[ "$($ENGINE exec e2e-panel stat -c %a $D/totp.json $D/secret.key $D/settings.json 2>&1 | tr '\n' ' ')" = "600 600 600 " ]; result $? "secret files are 0600"
[ "$($ENGINE exec e2e-panel head -c 7 $D/tls/panel.key)" = "enc:v1:" ]; result $? "the generated TLS key is sealed"
$ENGINE exec e2e-panel cat $D/ssh/agent | $ENGINE exec -i e2e-server sh -c 'cat > /tmp/k && grep -q "BEGIN OPENSSH PRIVATE KEY" /tmp/k && chmod 600 /tmp/k && ! ssh-keygen -y -P "" -f /tmp/k >/dev/null 2>&1'; result $? "the panel's SSH key does not open without its passphrase"
$ENGINE exec e2e-panel test -s $D/tls/panel.key && ! $ENGINE exec e2e-panel grep -rqi 'BEGIN .*PRIVATE KEY' $D/tls; result $? "no private key in plain PEM in the data directory"

[ "$STATUS" = 0 ] && [ $FAILED = 0 ]
