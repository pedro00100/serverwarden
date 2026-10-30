# Security

## Reporting a vulnerability

Please report it privately through GitHub's *Report a vulnerability* (Security tab of
this repository), not in a public issue. Include the version, what you did and what
happened. You will get an answer within a week; fixes are released as soon as they are
ready and credited to you unless you prefer otherwise.

Supported versions: the latest release. Run `docker compose pull` to stay on it.

## What the panel protects

The project is responsible for what it controls:

- **Access.** Passkeys (WebAuthn, user verification required) or Argon2id passwords with
  optional TOTP two-step verification and single-use recovery codes. Sessions expire
  after 30 minutes idle and 8 hours in total; settings stay locked until you confirm
  again. Sign-in attempts are rate-limited per address and notified.
- **Network exposure.** The panel only answers the networks you allow, over TLS 1.3,
  with strict security headers, same-origin and CSRF checks on every change.
- **Secrets at rest.** No secret is stored in plain text: notification tokens, the
  generated TLS key and the TOTP data are sealed with AES-256-GCM under a separate key
  (`secret.key` or a container secret), bound to where each value belongs; the panel's
  SSH keys use OpenSSH passphrase encryption; the TOTP secret is also sealed with a key
  derived from your password; passwords and recovery codes are only stored as hashes.
- **Secrets in transit.** No endpoint returns a stored secret. Settings come back
  masked, errors and logs are redacted, API answers carry `Cache-Control: no-store`.
  The TOTP secret is shown once, while you enable it, and its QR code is drawn by the
  panel, without third-party services.
- **Servers.** The panel reaches each server with SSH keys restricted to the agent
  (`restrict`, `command=`, optional `from=`), only after you confirm the host key.
  Actions are declared by the server itself and, with passkeys, signed for that action
  and verified by the server. Running your own commands or exporting images is off
  unless the server's root-owned `agent.json` allows it.
- **Supply chain.** The agent and Trivy are downloaded with pinned certificates or
  checksums. Dependencies are few, pinned and checked by `go mod verify`.

## What you are responsible for

The panel cannot protect the machines it runs on. As the owner you are responsible for:

- **The host.** Keep the operating system, Docker or Podman, and the panel image up to
  date. Anyone with root or physical access to the host can read everything the panel
  can, including `secret.key`.
- **The network.** Do not publish the panel to the internet. Reach it over your LAN or a
  VPN, and restrict its port with your firewall as well as with the allowed networks.
- **The container.** Run it as in the example compose file: non-root user, read-only
  filesystem, no capabilities, `no-new-privileges`, memory and process limits.
- **Backups.** Back up the data directory and `secret.key` (without the key the sealed
  values cannot be read), and protect those backups: encrypt them and keep the key
  apart when you can.
- **Your servers.** Decide in each `agent.json` what the panel may do there. Accepting
  panel commands means whoever controls the panel and your second factor can run
  anything on that server. Do not write passwords in commands or manual actions: they
  are shown in the UI and in notifications.
- **Your second factor.** Keep the recovery codes offline or in a password manager, and
  the security key or phone with you.
