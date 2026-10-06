# Where AI Badger Helps a Coding Agent: An 18-Run Follow-Up

The [earlier single-run article](../can-ai-badger-reduce-local-coding-agent-token-usage/index.md) found that a Badger-assisted compact handoff made one OpenCode session much smaller. This follow-up repeats the idea with 18 Codex coding sessions. It also separates "having a plan" from "having repository context."

Short version:

- **Badger context lowered uncached input in all six observed pairs, including one interrupted run,** against the same plan written without repository context. Across the five usable pairs the drop was −30.8%.
- **Every Badger-context implementation passed the independent checks.** It was the only workflow with six recorded successes out of six.
- **Badger's own step costs almost nothing.** Local design plus file extraction took about 0.36 seconds in total across all six runs. It made no model call and uploaded no repository.
- **Total coding tokens did not drop consistently.** Results depended on the task, and a plan on its own (without repository context) usually made the coding session larger, not smaller.

The cost of the web-chat planning step was not measured, so this is not a whole-workflow cost result.

## Setup

The fixture is a small Django and SQLite note application with a JSON API and automated tests. Each coding session started from the same frozen copy.

Three tasks:

1. **Title length.** Reject titles longer than 120 characters after trimming, on create, edit and import.
2. **Pinned notes.** Add a persisted `pinned` flag, including ordering, a migration and backup compatibility.
3. **Unchanged saves.** A save that doesn't really change a note must keep its `modified_at` timestamp and its sort position.

Three workflows:

```text
A  Direct           plan + code     coding agent

B  Generic plan     plan            web chat (task + one-line stack description)
                    code            coding agent

C  Badger context   choose files    web chat
                    read the repo   Badger, local, no model call
                    plan            web chat (with Badger context)
                    code            coding agent
```

B and C used the same planner and the same handoff shape. The only difference is that C's planner saw repository context extracted by Badger. **C versus B is the main comparison.**

- **Coding agent:** Codex CLI 0.160.0, `gpt-6-luna` at Medium.
- **Planner:** a fresh ChatGPT Plus chat (Sol-5.6 High).
- **Badger:** v0.6.1.

There were 3 tasks × 3 workflows × 2 repetitions, for 18 coding sessions. The schedule was fixed in advance and rotated workflow order. Failures were kept, with no selective retries.

## Result 1: Badger context cuts fresh input

Coding-agent input has two parts:

- **Cached input** is content the provider has already seen, such as repeated system prompts and repeated file reads. Providers usually discount it heavily.
- **Uncached input** is input minus cached input: material the provider did not serve from cache. The runs do not split that remainder into repository code, handoff text, and other new material.

| Task | Rep. | B uncached input | C uncached input | C vs B |
|---|---:|---:|---:|---:|
| Title length | 1 | 25,694 | 9,957 | −61.2% |
| Title length | 2 | 23,161 | 19,427 | −16.1% |
| Pinned notes | 1 | 30,352 | 26,817 | −11.6% |
| Pinned notes | 2 | 30,434 | 21,940 | −27.9% |
| Unchanged save | 1 | 32,265 | 20,124 | −37.6% |
| Unchanged save | 2 | 24,580* | 17,681 | −28.1%* |

\* B's coding run was interrupted, so this pair is excluded from the efficiency comparisons below. It is shown here only because it points the same way.

C's uncached input was lower in all six observed pairs, including the interrupted run. Across the five usable pairs it fell from 141,906 to 98,265 tokens (−30.8%).

Over all six runs per workflow:

| Workflow | Uncached input | Total coding tokens |
|---|---:|---:|
| A Direct | 148,976 | 676,701 |
| B Generic plan | 166,486 | 999,319 |
| C Badger context | 115,946 | 1,082,419 |

That drop is consistent with less repository rediscovery. A plan that already names the relevant files can leave the coding agent less new code to process. The capture did not classify uncached tokens by source, and provider caching was uncontrolled, so this is not an established mechanism. A generic plan did not show the same drop against direct prompting: B's uncached input was higher than A's.

Uncached input was not the endpoint chosen before the runs. Treat the consistent direction as the signal, not the exact percentage.

## Result 2: correctness held

Independent checkers were frozen before any coding run.

| Workflow | Independent checks | Recorded outcomes |
|---|---|---|
| A Direct | 5 pass, 1 fail | 5 success, 1 validation failure |
| B Generic plan | 6 pass | 5 success, 1 incomplete (interrupted, recovered code passed) |
| C Badger context | 6 pass | 6 success |

The single behaviour failure was a direct run on pinned notes. Importing an older backup without the `pinned` field did not preserve the existing pin, even though the request described that case. One miss doesn't prove that planning improves correctness, but the planned workflows lost nothing on correctness.

Several runs left an application test asserting old behaviour after the feature changed it. Those tests are treated as supporting evidence. The frozen checkers are the behaviour record.

## Result 3: total coding tokens depend on the task

This is the comparison the batch was designed for. It doesn't favour either workflow overall.

| Task | Rep. | C vs B coding tokens | C vs B coding time |
|---|---:|---:|---:|
| Title length | 1 | −26.3% | −37.4% |
| Title length | 2 | −32.0% | −16.8% |
| Pinned notes | 1 | +124.1% | −35.4% |
| Pinned notes | 2 | +27.3% | −15.6% |
| Unchanged save | 1 | −8.9% | +180.4% |
| Unchanged save | 2 | excluded | excluded |

- **Title length:** C was smaller and faster both times.
- **Pinned notes:** C was faster both times but used more total tokens. Almost all of the extra was cached input, and C's fresh input was still lower.
- **Unchanged save:** C used fewer tokens but took much longer.

A generic plan against direct prompting (B vs A) was mostly worse on the coding side. It used more coding tokens in 3 of 4 usable pairs and took longer in all 4. A plan with no repository context doesn't shrink the coding session. It just moves some of the thinking into the chat.

## What this means for using Badger

The useful pattern is:

1. Let Badger find and extract the relevant code locally. This is fast, uses no model and needs no upload.
2. Plan in a web chat with that context.
3. Give the coding agent a compact, concrete handoff.

What the data supports:

- The coding agent processed less uncached input in every observed C-versus-B pair, including the interrupted run. Across the five usable pairs the drop was −30.8%.
- Implementations were at least as correct as with direct prompting.
- Planning moved out of the coding agent's quota into a chat subscription.

What it doesn't support:

- A guaranteed smaller coding session.
- A lower total AI cost. The chat side wasn't measured.

For users whose coding-agent quota runs out before their chat quota does, moving the repository-aware planning step into chat is the practical gain. Badger is what makes that chat aware of the repository.

## Caveats

- Three tasks, two repetitions, one small app. These are descriptive results, not statistical claims.
- Web-chat tokens, latency and price were not recorded.
- Planner model revision and isolation were not independently verified.
- Provider caching was uncontrolled, and local caches were warm.
- The unchanged-save task turned out to need little repository discovery.
- One generic-plan run was interrupted. Its code was recovered and passed, but its efficiency pairs are excluded.
- One planning paste was first duplicated from a previous Badger handoff. It was discarded, and coding started from a fresh generic handoff.

## Data

- [runs.csv](./data/runs.csv) / [runs.json](./data/runs.json): all 18 coding sessions, with token components, duration and validation
- [paired-comparisons.csv](./data/paired-comparisons.csv) / [paired-comparisons.json](./data/paired-comparisons.json): B vs A and C vs B, with excluded pairs marked
- [fields.md](./data/fields.md): field definitions and token-accounting rules

## Conclusion

In this batch, adding Badger repository context to a compact plan did not make every coding session smaller. The coding agent processed less uncached input in every observed pair, which is consistent with less repository rediscovery, and every Badger-context implementation passed the independent checks. The local repository-reading step made no model call.

That is the honest case for Badger: it gives an external planner the repository context it needs without uploading the code, and in this batch the coding agent then processed less uncached input.
