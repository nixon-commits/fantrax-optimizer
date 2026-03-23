# CLAUDE.md

## Commands

```bash
go build ./...          # build all packages
go test ./internal/...  # run all unit tests (no network required)
go test ./internal/optimizer/...  # run a specific package's tests
go run ./cmd --dry-run  # run locally without applying changes
go run ./cmd --dry-run --date 2026-04-01  # test a specific date
```

Tests require no credentials — all network dependencies are mocked via interfaces or test servers.

For local dev, create a `.env` file (gitignored) with `FANTRAX_USERNAME`, `FANTRAX_PASSWORD`, `FANTRAX_LEAGUE_ID`, `FANTRAX_TEAM_ID`. Loaded automatically by `godotenv`.

## Architecture

The optimizer runs as a single binary (`cmd/main.go`) that wires together four independent packages:

```
fantrax client  ──┐
mlb schedule    ──┼──► optimizer ──► apply lineup (or dry-run print)
fangraphs proj  ──┘
```

**`internal/config`** — loads env vars via `godotenv`, validates that all four required vars are set, and returns a `Config` struct used by `cmd/main.go` to wire everything together.

**`internal/fantrax`** — wraps `github.com/pmurley/go-fantrax` (public read API) and `go-fantrax/auth_client` (authenticated API + lineup writes). Key details:
- `auth_client` uses chromedp (headless Chrome) to log in and obtain a session cookie. Cookie is cached in `.fantrax-cache/`. On first run or cache miss, a browser opens.
- Credentials read from env: `FANTRAX_USERNAME`, `FANTRAX_PASSWORD`, `FANTRAX_LEAGUE_ID`, `FANTRAX_TEAM_ID`.
- Alternatively, set `FANTRAX_COOKIES` to the raw `FX_RM` cookie value to skip browser login entirely.
- **Position IDs are numeric strings** (`"001"` = C, `"002"` = 1B, `"003"` = 2B, `"004"` = 3B, `"005"` = SS, `"008"` = INF, `"012"` = OF, `"014"` = UT). These come from the roster API and must be used as-is for slot assignment and eligibility checks.
- This league's active slot names: `C`, `1B`, `2B`, `3B`, `SS`, `INF`, `OF` (×4), `UT` (×3). Mapped in `posNameToID` in `client.go`.
- Scoring group code is `BASEBALL_HITTING` (not `HITTING`).

**`internal/projections`** — FanGraphs Steamer projections (primary) with rolling-stats fallback chained via `ChainedSource`. FanGraphs returns **JSON** (not CSV); player name field is `PlayerName`. The `Projection` struct includes derived stats (`Singles`, `XBH`, `TB`) that must be computed from raw fields before scoring.

**`internal/schedule`** — hits `statsapi.mlb.com` for today's game schedule. Returns a `map[string]bool` of playing MLB team abbreviations. The URL is a `var` (not `const`) to allow test overriding.

**`internal/optimizer`** — pure functions, no I/O. `OptimizeLineup` sorts by (hasGame DESC, expectedPts DESC) then greedily fills slots in order (positional first, UT last) using position ID eligibility. `EligibleForSlot` in `fantrax/client.go` handles UT (accepts any hitter) and INF (accepts any infield position ID).

**Scoring model** — this league scores: `1B`, `2B`, `3B`, `HR`, `RBI`, `R`, `BB`, `SB`, `CS`, `HBP`, `SO`, `GIDP`, `XBH`, `TB`, `CYC`. The `expectedPts` function derives `1B = H - 2B - 3B - HR`, `XBH = 2B + 3B + HR`, `TB = 1B + 2×2B + 3×3B + 4×HR` before applying weights.

## GHA

`.github/workflows/lineup.yml` runs daily at 10am UTC (6am ET) and on `workflow_dispatch`. Requires four repository secrets: `FANTRAX_USERNAME`, `FANTRAX_PASSWORD`, `FANTRAX_LEAGUE_ID`, `FANTRAX_TEAM_ID`. Chrome is installed via apt on `ubuntu-latest` before the Go run step.
