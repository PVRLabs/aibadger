# Privacy And Safety

Badger is local-first.

## Guarantees

- All scanning and extraction runs locally.
- No telemetry is collected.
- No cloud sync is used.
- No source code is copied until you approve the handoff.
- No file writes happen until you review the preview and confirm.

## Exclusions

Badger automatically excludes obvious secret-bearing and sensitive paths from scanning and extraction, including:

- **Credentials & Secrets**: `.env` (and `.env.*`), `.npmrc`, `.pypirc`, `.netrc`.
- **Keys & Certificates**: `*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa`, `id_dsa`, and other common private key formats.
- **Cloud Configs**: `.aws/credentials`, `.aws/config`, `.gcp/credentials.json`, `.azure/` directories.
- **System & Internal**: `.git`, `.kubeconfig`, and binary artifacts.

These exclusions are hard-coded in the engine to ensure that even if you submit a broad goal, sensitive local data remains local.

## Consent Model

Badger only copies content and applies file changes after explicit user confirmation.

## External Context

Read-only external directories can be listed in `.badger-context`.
They are summarized separately from the main project and cannot be used as patch targets.
