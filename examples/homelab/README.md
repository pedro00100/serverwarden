# Example: Juniper Lab

A made-up homelab that shows a complete configuration. Every name, address and
domain here is fictional (documentation ranges and `.example`).

```
            internet
               │
        ┌──────┴──────┐  WireGuard 10.99.0.0/24
        │ relay (VPS) │──────────────────────┐
        └─────────────┘                      │
                                   home 10.20.0.0/24
                         ┌───────────────────┴───────────────────┐
                    ┌────┴────┐                            ┌─────┴──────┐
                    │  atlas  │  Jellyfin, Home Assistant  │   pantry   │  NAS, restic,
                    └─────────┘                            └────────────┘  and the panel
```

| File | Goes to |
|---|---|
| `compose.yaml` | pantry, e.g. `/srv/panel/compose.yaml` |
| `settings.json` | optional: `docker compose run --rm panel -import /path/settings.json`, instead of the first-run wizard |
| `agent-relay.json`, `agent-atlas.json`, `agent-pantry.json` | each server's `/etc/serverwarden/agent.json` (root, 0600) |

Notes:

- `relay` keeps `panel_commands` off: it faces the internet, so only the two
  actions it declares itself can run there.
- `atlas` and `pantry` accept the commands defined in Settings › Commands.
- `pantry` reports its restic repositories, so backups show up in Security.
- `pantry` sets `image_export`: the panel image is built there and is in no registry,
  so this is the only way to scan it. The other servers run images pulled from
  registries, which are scanned without it.
- With `"access": {"mode": "passkey"}` the first key is registered with
  `docker exec panel serverwarden -new-enrollment`, which prints a one-time link.
