# Privacy And Safety

Badger prepares context locally. Nothing is uploaded automatically. Your AI provider receives what you paste.

## Guarantees

- All scanning and extraction runs locally.
- No telemetry is collected.
- No cloud sync is used.
- Sharing with an AI provider is explicitly user-controlled.
- No file writes happen until you review the preview and confirm.

## Exclusions

Badger excludes obvious secret-bearing and sensitive paths from supplemental scanning and extraction, including:

- **Credentials & Secrets**: `.env` and most `.env.*` files, `.npmrc`, `.pypirc`, `.netrc`. Common environment template files such as `.env.example`, `.env.template`, and `.env.sample` may be extracted.
- **Keys & Certificates**: `*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa`, `id_dsa`, and other common private key formats.
- **Cloud Configs**: `.aws/credentials`, `.aws/config`, `.gcp/credentials.json`, `.azure/` directories.
- **System & Internal**: `.git`, `.kubeconfig`, and binary artifacts.

These exclusions are path-based, not general secret detection. They do not filter the authoritative tracked Git diff used for review, which may include sensitive paths and their contents. Secrets can also appear in ordinary source files. Check the prepared context before sharing it.

## Consent Model

You choose when to copy prepared context and whether to paste it into an AI chat. File changes require preview and confirmation.

## External Context

Read-only external directories can be listed in `.badger-context`.
They are summarized separately from the main project and cannot be used as patch targets.
