# ACE (Agentic Context Engineering) in Crush

This fork includes an optional, native ACE-style “project memory” injector.

## What it does

- Stores a project-local playbook at `options.data_directory/ace/playbook.json` (default: `.crush/ace/playbook.json`).
- When enabled, injects a short “ACE Memory” block as an extra system prefix on each prompt.
- On exit (optional), summarizes the latest session and updates the playbook using a multi-step pipeline (extract → score → merge → cleanup).

In this fork, ACE memory injection is enabled by default; disable it explicitly if you don’t want it.

## Enable it

Add to your `crush.json`:

```json
{
  "options": {
    "ace": {
      "enabled": true,
      "update_on_exit": true,
      "max_items": 6,
      "min_score": 0,
      "max_chars": 2000,
      "playbook_path": "ace/playbook.json"
    }
  }
}
```

## Disable it

```json
{
  "options": {
    "ace": { "enabled": false }
  }
}
```

## Manage playbook

- `crush ace init`
- `crush ace add --text "Prefer gofumpt formatting" --tags go,format --score 1`
- `crush ace ls`
