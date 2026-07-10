## Base App Prompt

```text
Build a small local note-taking app in plain HTML, CSS, and JavaScript.

Use no framework, no build system, and no dependencies.

Requirements:
- Notes have id, title, body, tags, createdAt, and updatedAt.
- Support creating, editing, deleting, and selecting notes.
- Support searching notes by title/body.
- Support filtering notes by tag.
- Support sorting notes by updated newest, updated oldest, and title A-Z.
- Persist notes in localStorage.
- Persist settings in localStorage.
- Settings panel should include theme: light, dark, or system.
- Restore the selected note, search text, active tag filter, and sort order after page reload.
- Use a responsive layout that works on desktop and mobile.
- Keep the code small and readable.

Decide the implementation approach yourself.
```

## Workflow A Prompt

```text
Add a settings panel to this existing note-taking app.

Requirements:
- Add theme setting: light, dark, or system.
- Add sort order setting: newest updated, oldest updated, title A-Z.
- Persist settings in localStorage.
- Apply settings on page load.
- Preserve existing note create/edit/delete/search behavior.
- Keep the app plain HTML/CSS/JS with no dependencies.
```

## Workflow B Prompt

```text
Design the complete implementation, then produce a compact local-agent implementation plan with concrete changes only. Omit rationale, risks, open questions, acceptance criteria, and broad testing notes. Optimize the output for low token usage.
```

> Note: Badger `/design` captured project context first, then an external AI chat compressed that context into this compact handoff for OpenCode.
