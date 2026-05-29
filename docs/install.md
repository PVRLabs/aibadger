# Install AI Badger

This document covers install and build paths for the public OSS binary.

## Homebrew

Install from the public tap:

```bash
brew tap pvrlabs/aibadger
brew install pvrlabs/aibadger/badger
```

The tap pulls release tarballs from GitHub Releases.

## Curl Installer

Install the latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.sh | sh
```

The installer downloads the matching GitHub Release tarball for your platform,
verifies its SHA-256 checksum, and installs `badger` into `~/.local/bin` by
default. If that directory is not on your `PATH`, add it before running
`badger`.

Install a specific release:

```bash
curl -fsSL https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.sh | BADGER_VERSION=v0.2.0 sh
```

Install into a custom directory:

```bash
curl -fsSL https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.sh | BADGER_INSTALL_DIR="$HOME/bin" sh
```

## Windows

PowerShell one-liner (default install to `%LOCALAPPDATA%\Programs\Badger`):

```powershell
irm https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.ps1 | iex
```

Custom directory or version:

```powershell
$env:BADGER_INSTALL_DIR = "$HOME\bin"; irm https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.ps1 | iex
$env:BADGER_VERSION = "v0.2.0"; irm https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.ps1 | iex
```

Manual: download `badger_<version>_windows_amd64.zip` from the [latest release](https://github.com/PVRLabs/aibadger/releases/latest) and extract `badger.exe` to your `PATH`.

Build from source:

```powershell
go install github.com/PVRLabs/aibadger/cmd/badger@latest
```

## Build From Source

Development build:

```bash
go build -o badger ./cmd/badger
```

Release-style build:

```bash
go build -tags aibadger_release -ldflags="-s -w" -o badger ./cmd/badger
```

## Verify

Check the binary version:

```bash
./badger --version
```

Expected output:

```text
badger v0.2.0
```

Published installs should report the current release version. Source builds from
`main` may report the next development version, for example `badger v0.2.1-dev`,
until the next release is prepared.

## Release Notes

For release publishing and artifact details, see [releasing.md](releasing.md).
