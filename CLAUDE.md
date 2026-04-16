# CLAUDE.md — StremThru Project Context

## User Profile
- **User:** Justin (GitHub: hossman39)
- **Role:** Server admin managing a Stremio streaming stack for 60+ clients
- **Location:** Dunlap, Tennessee (all times in EST)
- **ISP:** Bledsoe Telephone Cooperative
- **Preferences:** See global `CLAUDE.md` in parent directory for principles, response style, security rules, and agent architecture.

## Project Overview

**StremThru** is a Stremio companion proxy (Go backend) that interfaces with debrid services (Torbox, RealDebrid, AllDebrid, etc.). This is a **fork** of [MunifTanjim/stremthru](https://github.com/MunifTanjim/stremthru).

- **Fork:** https://github.com/hossman39/stremthru
- **Upstream:** https://github.com/MunifTanjim/stremthru
- **Local path:** `C:\Users\Justin\stremthru`
- **Branch:** `main` (fork-specific commits on top of upstream main)

### Fork Customization: Torbox API Key Pool

The fork adds a **multi-key Torbox API pool with sticky selection and failover** (`store/torbox/keypool.go`) to distribute API usage across multiple Torbox accounts, preventing rate limit exhaustion (60 creates/hr, 10 creates/min per key).

**Key pool files (6 files, +395 lines vs upstream):**
| File | Purpose |
|------|---------|
| `store/torbox/keypool.go` | Key pool with usage tracking, sticky selection, failover |
| `store/torbox/keypool_config.go` | Pool configuration |
| `store/torbox/client.go` | Client modifications for pool integration |
| `store/torbox/store.go` | Store-level pool routing |
| `store/torbox/log.go` | Visual logging for key usage |
| `Dockerfile.keypool` | Separate Dockerfile for key pool builds |

**Recent commit history on fork:**
1. `bbb417c` — fix: use sticky key selection with failover instead of round-robin
2. `8dc8e99` — fix: resolve pool key once per operation, not per outbound call
3. `948078e` — fix: remove invalid shell syntax from Dockerfile COPY
4. `2b3b5fd` — fix: correct Dockerfile.keypool COPY paths for dash frontend
5. `4caa73b` — feat: add Torbox API key pool for multi-key load balancing

### Current Status

**StremThru key pool is the active key management layer; AIOStreams handles routing through it.**

- **StremThru key pool container** (`stremthru-keypool`, port 8082) is running on production
- The AIOStreams fork (hossman39/AIOStreams) implements key selection at the addon URL level — it selects a key from the StremThru pool and bakes it into all downstream calls (Comet, Torz, NZBHydra)

## Broader Streaming Infrastructure

### Production Server
- **IP:** 74.208.78.216 (IONOS VPS, Ubuntu 24.04)
- **SSH:** Key-based only (password auth disabled 2026-04-01)
  - Key: `C:/Users/Justin/.ssh/server_74.208.78.216`
  - `ssh -i ~/.ssh/server_74.208.78.216 root@74.208.78.216`
- **Management:** Portainer
- **VPS Firewall:** Allowlist-only (ports: 22, 80, 443, 81, 3000, 5901, 8000, 8001, 8008, 8009, 8443, 8447, 8765, 9443)

### Stack Components
| Service | Container | Port | Notes |
|---------|-----------|------|-------|
| AIOStreams (prod) | aiostreams-main | 3001 | Upstream nightly image |
| AIOStreams (test) | aiostreams-test | 3002 | Testing |
| AIOStreams (beta) | aiostreams-keypool | 3003 | Fork with key pool (stack aio-beta, compose dir 141) |
| Comet (test) | comet-test | 2024 | Stack 138 |
| StremThru (keypool) | stremthru-keypool | 8082 | Go key pool (this fork) |
| + Comet, NZBHydra, Easynews++, NPM | various | various | Production services |

### AIOStreams Fork (hossman39/AIOStreams)
- **Key pool feature:** Multi-key Torbox API pool with sticky selection and failover at the `getServiceCredential` level
- **Key files modified:** `torbox-keypool.ts`, `preset.ts`, `wrapper.ts`, `constants.ts`, `debrid/index.ts`, `api/debrid.ts`
- **3 Torbox API keys** in pool
- **All unit tests pass**, live traffic tests pass
- **Known issue:** Key selection fires 3x per request (once per addon). Do not add additional selection calls outside the existing addon entry points.
- **Workflow:** All changes go to `beta` branch first → test on `aiostreams-keypool` (port 3003) → merge to `main` only with explicit approval

### Torbox API Keys
- Key 1: `bc7f6473-...` (yes37824@gmail.com, id: 294741)
- Key 2: `cbeb26e3-...` (sdf.roulette269@passmail.net, id: 476910)
- Key 3: `59772855-...` (adf.vacate856@passmail.net, id: 478598)

## Key Decisions Made

1. **AIOStreams performs key selection from the StremThru pool (preferred)** — it picks a pool key and bakes it into all addon URLs, ensuring consistent key usage per addon
2. **Sticky key selection with failover** instead of pure round-robin — prevents key thrashing on retries
3. **Pool key resolved once per operation** — not per outbound call, reducing unnecessary rotation
4. **No infrastructure changes without permission** — always deploy as new test containers
5. **Docker builds happen locally, NEVER on production server**
6. **Beta branch workflow** — all AIOStreams changes go to beta first; production stays on main

## What's Planned Next

- Optimize AIOStreams key selection to fire once per user stream request (currently 3x per addon)
- Fix usage count inflation (counts addon URL builds, not actual Torbox API calls)
- Periodic upstream merge (StremThru upstream is actively developed)
- Evaluate whether StremThru key pool container can be decommissioned if AIOStreams key selection proves sufficient

## Related Projects

- **TV Box Manager** (`C:\Users\Justin\Downloads\TV_Box_Manager\tv_box_manager\`) — PyQt5 desktop app for Android TV box ADB automation (~90% complete, all scripts working, 51% perf optimization achieved)
- **TV Box CRM** (`C:\Users\Justin\Desktop\tv-box-crm\`) — FastAPI + React app for tracking TV box inventory
- **AIOManager** — Manages ~60 client Stremio accounts server-side

## Build & Deploy Notes

### StremThru (this repo)
```bash
# Build locally:
docker buildx build -f Dockerfile.keypool -t stremthru-keypool:latest --load .
# Transfer to server, redeploy via Portainer
```

### AIOStreams (beta)
```bash
# Local build:
cd /tmp/aiostreams-build && git pull
docker buildx build -t aiostreams-keypool:latest --load .
# Deploy via Portainer stack aio-beta (compose dir 141)
```

## Known Pitfalls
- **Pool keys are sticky with failover (no round-robin). Resolve once per operation, not per outbound call.**
- DO NOT touch upstream files unless merging — fork changes live in the 6 key pool files only
- Key selection fires 3x per request (once per addon). Do not add additional selection calls outside the existing addon entry points.

## Guardrails
- Never modify running production containers — deploy as new test containers
- Never build Docker images on the production server — build locally, transfer via save/load
- Never push directly to AIOStreams `main` branch — all changes go to `beta` first
- Always test on beta/test containers before production
- Report all times in EST

## Verify
```bash
go build ./... && go test ./... && echo "OK"
```
