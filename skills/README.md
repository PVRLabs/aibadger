# AI Badger Agent Skills

[![skills.sh](https://skills.sh/b/PVRLabs/aibadger)](https://skills.sh/PVRLabs/aibadger)

AI Badger provides two skills that let a coding agent carry useful conversation
state into Badger:

- `handoff` continues an active coding, debugging, planning, or architecture
  session in Badger.
- `badger-review` asks Badger for an independent review of work from the current
  session.

The skills save the current goal, recent decisions, constraints, completed
work, and verification in a local `.badger-handoff` file. Badger then combines
that session state with repository context it collects locally.

## Install

Install the Badger CLI first. See the [installation guide](../docs/install.md).

Then install both bundled skills without network access:

```bash
badger skills install
```

Alternatively, install them through [skills.sh](https://skills.sh/PVRLabs/aibadger):

```bash
npx skills add PVRLabs/aibadger
```

The skills and CLI are separate installs. Installing the skills does not
install the `badger` command.

## Use

Ask your coding agent naturally. For example:

```text
Hand this session off to Badger.
```

Or request an independent review:

```text
Prepare this work for a Badger review.
```

The agent writes `.badger-handoff` in the current workspace and gives you the
exact command to run in a separate terminal:

```bash
badger continue
```

Badger reads and removes the handoff file after accepting it.

## What gets transferred

The handoff file contains a compact summary of the current agent conversation.
The skills do not inspect repository files, Git history, diffs, or project
topology. Badger collects the repository context itself when it starts.

Review the individual skill definitions for their exact behavior:

- [`handoff`](handoff/SKILL.md)
- [`badger-review`](badger-review/SKILL.md)
