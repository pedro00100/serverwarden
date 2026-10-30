#!/bin/sh
# End-to-end test of the panel binary inside a throwaway Alpine container:
# first-run wizard, sign-in, settings security, adding a real SSH server with
# the install script, recovery mode and the CLI tools.
set -u
apk add --no-cache openssl curl openssh-server openssh-client jq >/dev/null 2>&1
cd /tmp
P=/out/serverwarden
FAILED=0
ok() { echo "PASS $1"; }
bad() { echo "FAIL $1"; FAILED=1; }
check() { if eval "$2"; then ok "$1"; else bad "$1"; fi; }

# --- First run: setup wizard ----------------------------------------------
mkdir -p data
$P -data-dir data -agents-dir /out/agents >panel.log 2>&1 &
PID=$!
sleep 2
CODE=$(grep -o 'setup=[A-Za-z0-9_-]*' panel.log | head -1 | cut -d= -f2)
check "setup link logged" '[ -n "$CODE" ]'
U=https://localhost:8443
C="curl -4sk -b jar -c jar -H Origin:$U -H X-Panel:1 -H Content-Type:application/json"
check "setup mode" '$C $U/api/session | grep -q "\"setup\":true"'
check "setup needs the link" '[ "$($C -o /dev/null -w %{http_code} $U/api/setup/status)" = 401 ]'
check "setup link accepted" '$C -X POST $U/api/setup/login -d "{\"code\":\"$CODE\"}" | grep -q ok'
check "setup suggests networks" '$C $U/api/setup/status | grep -q "127.0.0.1/32"'
SETUP='{"language":"es","title":"Smoke","origin":"https://localhost:8443","listen":"127.0.0.1:8443","allowed":["127.0.0.0/8"],"certificate":{"mode":"generate"},"username":"admin","password":"una contraseña larga"}'
check "weak password refused" '$C -X POST $U/api/setup/complete -d "$(echo "$SETUP" | sed s/una\ contraseña\ larga/short/)" | grep -q password_policy'
check "lockout refused" '$C -X POST $U/api/setup/complete -d "$(echo "$SETUP" | sed s#127.0.0.0/8#10.0.0.0/8#)" | grep -q lockout'
check "setup completed" '$C -X POST $U/api/setup/complete -d "$SETUP" | grep -q "\"ok\":true"'
sleep 3
check "certificate generated (key 600)" '[ "$(stat -c %a data/tls/panel.key)" = 600 ]'
check "panel SSH keys generated" '[ -f data/ssh/agent ] && [ "$(stat -c %a data/ssh/agent)" = 600 ]'
check "normal mode after setup" '$C $U/api/session | grep -q "\"mode\":\"password\""'

# --- Sign-in and settings security ----------------------------------------
check "wrong password rejected" '[ "$($C -o /dev/null -w %{http_code} -X POST $U/api/login/password -d "{\"username\":\"admin\",\"password\":\"wrong wrong wrong\"}")" = 401 ]'
check "login" '[ "$($C -o /dev/null -w %{http_code} -X POST $U/api/login/password -d "{\"username\":\"admin\",\"password\":\"una contraseña larga\"}")" = 200 ]'
check "hash hidden" '! $C $U/api/settings | grep -q argon2'
check "locked by default" '$C -X PUT $U/api/settings/general -d "{\"title\":\"X\",\"description\":\"\",\"language\":\"es\",\"interval_sec\":15}" | grep -q locked'
check "unlock" '$C -X POST $U/api/unlock/password -d "{\"password\":\"una contraseña larga\"}" | grep -q unlocked_until'
CH='{"alerts":{"pending_sec":60,"repeat_min":0,"resolved":true},"channels":[{"id":"d","name":"Discord","type":"discord","enabled":false,"min_level":"info","events":[],"template":"","settings":{"url":"https://discord.com/api/webhooks/1/TOPSECRET"}}]}'
check "channel saved" '$C -X PUT $U/api/settings/notifications -d "$CH" | grep -q "\"ok\":true"'
check "secret masked" '! $C $U/api/settings | grep -q TOPSECRET'
MASKED=$($C $U/api/settings | jq -c '.settings.notifications')
check "masked secret kept on save, sealed on disk" '$C -X PUT $U/api/settings/notifications -d "$MASKED" >/dev/null && grep -q "\"url\": \"enc:v1:" data/settings.json && ! grep -q TOPSECRET data/settings.json data/settings.json.bak'
check "test error redacted" '! $C -X POST $U/api/settings/notifications/test -d "{\"id\":\"x\",\"name\":\"t\",\"type\":\"discord\",\"settings\":{\"url\":\"https://127.0.0.1:1/api/webhooks/1/OTHERSECRET\"}}" | grep -q OTHERSECRET'
check "lockout prevented" '$C -X PUT $U/api/settings/web -d "{\"listen\":\"127.0.0.1:8443\",\"origin\":\"https://localhost:8443\",\"certificate\":\"/tmp/data/tls/panel.crt\",\"key\":\"/tmp/data/tls/panel.key\",\"allowed\":[\"10.0.0.0/8\"]}" | grep -q lockout'
check "csrf: foreign origin" '[ "$(curl -4sk -b jar -o /dev/null -w %{http_code} -X PUT $U/api/settings/general -H Origin:https://evil.example -H X-Panel:1 -H Content-Type:application/json -d "{}")" = 403 ]'
check "csrf: missing header" '[ "$(curl -4sk -b jar -o /dev/null -w %{http_code} -X PUT $U/api/settings/general -H Origin:$U -H Content-Type:application/json -d "{}")" = 403 ]'

# --- Add a real server -------------------------------------------------------
rm -f /usr/bin/ssh   # the panel speaks SSH itself
ssh-keygen -A >/dev/null
sed -i 's/^#*PermitRootLogin.*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config
passwd -d root >/dev/null 2>&1 || true
/usr/sbin/sshd -p 2222 -o ListenAddress=127.0.0.1
sleep 1
HK=$($C -X POST $U/api/settings/servers/hostkey -d '{"host":"127.0.0.1","port":2222}')
REAL=$(ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub | cut -d" " -f2)
check "host key fingerprint matches sshd" '[ "$(echo "$HK" | jq -r .fingerprint)" = "$REAL" ]'
SRV='{"original_id":"","server":{"id":"box","name":"box","short":"BOX","role":"Test","location":"Here","layer":"internal","ssh":{"host":"127.0.0.1","port":2222,"user":"root"}},"host_key":""}'
check "unconfirmed host key refused" '$C -X PUT $U/api/settings/servers -d "$SRV" | grep -q host_key'
SRV=$(echo "$SRV" | jq -c --arg k "$(echo "$HK" | jq -r .key)" '.host_key=$k')
check "server saved" '$C -X PUT $U/api/settings/servers -d "$SRV" | grep -q "\"ok\":true"'
check "host key pinned" 'grep -q "\[127.0.0.1\]:2222 ssh-ed25519" data/ssh/known_hosts'
SCRIPT=$($C -X POST $U/api/settings/servers/box/install -d "{\"panel_url\":\"$U\",\"from\":\"\"}" | jq -r .script)
check "install script issued" 'echo "$SCRIPT" | grep -q pinnedpubkey'
echo "$SCRIPT" > install.sh
check "install script runs" 'sh install.sh > install.log 2>&1'
check "install prints fingerprints" 'grep -q "$REAL" install.log'
check "agent installed" '[ -x /usr/local/bin/serverwarden-agent ]'
check "forced commands in authorized_keys" '[ "$(grep -c "^restrict,command=\"/usr/local/bin/serverwarden-agent" /root/.ssh/authorized_keys)" = 2 ]'
check "agent.json root 600" '[ "$(stat -c %a /etc/serverwarden/agent.json)" = 600 ]'
check "script is idempotent" 'sh install.sh >/dev/null 2>&1 && [ "$(grep -c serverwarden- /root/.ssh/authorized_keys)" = 2 ]'
BADPIN=$(echo "$SCRIPT" | sed "s#sha256//[A-Za-z0-9+/=]*#sha256//AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=#")
rm -f /usr/local/bin/serverwarden-agent
check "wrong certificate pin aborts the download" '! sh -c "$BADPIN" >/dev/null 2>&1 && [ ! -e /usr/local/bin/serverwarden-agent ]'
sh install.sh >/dev/null 2>&1
check "connection test" '$C -X POST $U/api/settings/servers/box/test | grep -q "\"ok\":true"'
sleep 1
for i in 1 2 3 4 5 6 7 8 9 10; do $C $U/api/state | jq -e '.view.servers[0].snapshot.hostname' >/dev/null 2>&1 && break; sleep 2; done
check "live monitoring without restart" '$C $U/api/state | jq -e ".view.servers[] | select(.server.id==\"box\") | .snapshot.cores > 0" >/dev/null'
check "agent actions listed" '$C $U/api/actions | jq -e ".actions | type == \"array\"" >/dev/null'
CMD='[{"id":"hello","name":"Hello","description":"","command":"echo hi from $(hostname)","servers":[]}]'
check "command saved" '$C -X PUT $U/api/settings/commands -d "$CMD" | grep -q "\"ok\":true"'
check "command refused by a server that did not opt in" '! $C -X POST $U/api/action/run -d "{\"server\":\"box\",\"action\":\"cmd:hello\"}" | grep -q "\"ok\":true"'
check "panel commands off by default" 'grep -q "\"panel_commands\": false" /etc/serverwarden/agent.json'
sed -i 's/"panel_commands": false/"panel_commands": true/' /etc/serverwarden/agent.json
for i in $(seq 1 20); do $C $U/api/actions | jq -e '.actions[] | select(.id=="cmd:hello")' >/dev/null 2>&1 && break; sleep 2; done
check "command offered once the server opts in" '$C $U/api/actions | jq -e ".actions[] | select(.id==\"cmd:hello\" and .server==\"box\")" >/dev/null'
OUT=$($C -X POST $U/api/action/run -d '{"server":"box","action":"cmd:hello"}')
check "command runs over SSH and returns its output" 'echo "$OUT" | jq -e ".ok and (.output | startswith(\"hi from\"))" >/dev/null'
rm -f /etc/ssh/ssh_host_*; ssh-keygen -A >/dev/null; kill "$(cat /run/sshd.pid)"; sleep 1; /usr/sbin/sshd -p 2222 -o ListenAddress=127.0.0.1; sleep 1
R=$($C -X POST $U/api/settings/servers/box/test)
check "changed host key refused" 'echo "$R" | grep -q "HOST KEY CHANGED"'
check "server removed and unpinned" '[ "$($C -o /dev/null -w %{http_code} -X DELETE $U/api/settings/servers/box)" = 200 ] && ! grep -q 2222 data/ssh/known_hosts'
check "secrets never logged" '! grep -q -e TOPSECRET -e OTHERSECRET panel.log'
kill $PID; wait $PID 2>/dev/null

# --- Recovery mode -----------------------------------------------------------
echo '{broken' > data/settings.json
$P -data-dir data -setup-listen 127.0.0.1:8444 >rec.log 2>&1 &
PID=$!
sleep 2
RCODE=$(grep -o 'recover=[A-Za-z0-9_-]*' rec.log | head -1 | cut -d= -f2)
R="curl -4sk -b jar2 -c jar2 -H Origin:https://localhost:8444 -H X-Panel:1 -H Content-Type:application/json"
check "recovery link logged" '[ -n "$RCODE" ]'
check "recovery needs the link" '[ "$($R -o /dev/null -w %{http_code} https://localhost:8444/api/recovery/status)" = 401 ]'
check "recovery login" '$R -X POST https://localhost:8444/api/recovery/login -d "{\"code\":\"$RCODE\"}" | grep -q ok'
check "backup restored" '$R -X POST https://localhost:8444/api/recovery/restore-backup | grep -q ok'
$R -X POST https://localhost:8444/api/recovery/restart >/dev/null; sleep 3
check "back to normal" '$C $U/api/session | grep -q "\"mode\":\"password\""'
kill $PID 2>/dev/null; wait $PID 2>/dev/null

# --- CLI ---------------------------------------------------------------------
mkdir -p cli
check "reset-password" 'cp data/settings.json cli/ && echo "otra contraseña larga" | $P -data-dir cli -reset-password >/dev/null 2>&1 && grep -q argon2id cli/settings.json'

echo "---"
if [ $FAILED = 0 ]; then echo "ALL PASSED"; else echo "SOME FAILED"; tail -15 panel.log; cat install.log 2>/dev/null | tail -5; fi
exit $FAILED
