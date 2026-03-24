---
name: Strategic audit findings March 2026
description: Key findings from comprehensive codebase audit — critical bugs, strategic gaps, and prioritized improvements
type: project
---

Comprehensive audit completed 2026-03-23. Key findings by priority:

**Critical:** RollingSource (rolling.go) omits SO and GIDP parameters, inflating fallback projections. Hitter blending has no min-GP threshold (pitcher blending requires 4).

**Important:** expectedPts logic is duplicated between optimizer and projections packages (circular import avoidance). Blend weights (0.60/0.40, 0.85/0.15, 0.70/0.30) and lookback window (10 periods) are hardcoded constants with no env var override. Non-starting SP discount of 10% is likely too generous — should be 1-5%. GetLeagueInfo is called 4 times redundantly per run. GHA workflows lack timeout-minutes. No test coverage for cmd layer, pitcher blended source, NormalizeTeam, or config package.

**Why:** These collectively affect projection accuracy for edge-case players, runtime efficiency, and safety for unattended daily runs.

**How to apply:** When modifying projections or scoring logic, always update both the optimizer and projections copies. When adding new scoring categories, add them to both statMaps. Prioritize the hitter min-GP threshold and RollingSource SO/GIDP fixes as highest-impact improvements.
