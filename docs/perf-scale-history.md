# Clack memory at Hack Club scale

One row per platform per run of perf.yml's scale jobs (fakeslackd -scale: 65,846 emoji entries, 32k users, 2,006 user groups, 300 channels, long channels with images). The stages are the driver's (internal/ui/perfdrive.go); memory is the private working set on Windows (Task Manager's Memory column; no GPU on the runner: the Basic Render Driver) and private RSS on Linux (llvmpipe).

| date | commit | branch | platform | startup | picker scrolled | long channels | +40 channels | idle 10 min | idle Go heap | idle stacks |
|---|---|---|---|---|---|---|---|---|---|---|
| 2026-11-23 | `1449eb6` | main | linux | 176 MB | 517 MB | 824 MB | 549 MB | 470 MB | 289 MB | 1 MB |
| 2026-11-23 | `1449eb6` | main | windows | 204 MB | 220 MB | 682 MB | 497 MB | 517 MB | 251 MB | 1 MB |
