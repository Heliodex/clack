# Clack perf history

One row per push (perf workflow, GitHub runner, llvmpipe). Paint and RSS are the real binary on Xvfb against fakeslackd.

| date | commit | branch | warm first paint | cold first paint | idle RSS | session heap | channel switch | scroll frame | typing frame / allocs | picker open | binary |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 2026-11-23 | `1449eb6` | main | 263 ms | 267 ms | 312 MB | 108 MB | 127.61 ms | 68.68 ms | 65.61 ms / 1,935 | 152.96 ms | 81.6 |
