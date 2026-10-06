# Curated result fields

Publication-oriented extract of the 18 coding attempts. Derived from the retained normalized coding-session export by keeping task, workflow, outcome, token components, duration, and validation fields. Session identifiers, artifact hashes, filesystem paths, environment identifiers, and run-record names are omitted.

The source records intentionally remain `comparable=false`. Whole-workflow tokens, web-chat usage, provider price, and true model-generation latency were unavailable. This extract does not flip that flag.

## `runs.csv` and `runs.json`

| Field | Meaning |
|---|---|
| `task` | Task number: 1 title length, 2 pinned notes, 3 unchanged save. |
| `task_slug` | Stable name: `title-length`, `pinned-notes`, `unchanged-saves`. |
| `task_name` | Short display name. |
| `repetition` | 1 or 2. |
| `workflow` | `A` direct, `B` generic web-chat plan, `C` Badger context plus web-chat plan. |
| `schedule_position` | Fixed execution order, 1 through 18. |
| `outcome` | `success`, `validation_failure`, or `incomplete`. |
| `coding_tokens` | Coding-agent total for that Codex session. Equals `input_tokens + output_tokens`. |
| `input_tokens` | Reported input. Includes `cached_input_tokens`. |
| `cached_input_tokens` | Cached portion of input. Already inside `input_tokens`, so it is not added again. |
| `output_tokens` | Reported output. Includes `reasoning_tokens`. |
| `reasoning_tokens` | Reasoning portion of output. Already inside `output_tokens`, so it is not added again. |
| `coding_duration_seconds` | Elapsed coding-session time after the implementation input. Empty or null when unavailable. This is not model-generation latency and does not include web-chat planning. |
| `independent_validation` | Frozen independent feature check: `pass` or `fail`. |
| `app_tests` | The application's own tests after the coding session: `pass` or `fail`. Supporting evidence. A failure here is not automatically an independent feature miss. |

The article's uncached-input figures are derived as `input_tokens - cached_input_tokens`. That is a post-hoc view, not a field in the source export.

Repeated cumulative usage counters were not summed. Web-chat tokens are absent because they were not available. They are not zero.

## `paired-comparisons.csv` and `paired-comparisons.json`

Each row is one scheduled pair. `B vs A` asks whether a generic web-chat plan changed the coding session relative to asking Codex directly. `C vs B` asks whether adding Badger repository context changed the coding session relative to that generic plan. `C vs B` is the primary comparison.

`coding_token_change_percent` and `coding_duration_change_percent` are `(candidate / baseline - 1) * 100`. Negative means the candidate used fewer coding-agent tokens or less coding time. The article rounds included percentages to one decimal place.

`included_in_efficiency_comparison` is true only when both outcomes are `success` and both coding durations are present. Otherwise the percentage fields are null, and `exclusion_reason` says why. Excluded rows still carry the raw coding-token counts so the exclusion is visible. Those raw counts are not an efficiency result.

The three excluded pairs are:

- Pinned notes, repetition 2, B vs A: the direct run failed independent validation.
- Unchanged save, repetition 2, B vs A: the generic-planning coding run was interrupted.
- Unchanged save, repetition 2, C vs B: the same interrupted generic-planning run. Its coding duration is unavailable.
