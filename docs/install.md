# Install AIBadger

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
curl -fsSL https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.sh | BADGER_VERSION=v0.1.3 sh
```

Install into a custom directory:

```bash
curl -fsSL https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.sh | BADGER_INSTALL_DIR="$HOME/bin" sh
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
badger v0.1.3
```

Published installs should report the current release version. Source builds from
`main` may report the next development version, for example `badger v0.1.4-dev`,
until the next release is prepared.

## Windows

Install on Windows via PowerShell, manual download, or source build.

### PowerShell one-liner

```powershell
irm https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.ps1 | iex
```

The installer downloads the matching GitHub Release `.zip` for Windows and
places `badger.exe` into `%USERPROFILE%\.local\bin` by default. If that
directory is not on your `PATH`, the installer prints instructions.

Override the install directory:

```powershell
$env:BADGER_INSTALL_DIR = "$HOME\bin"
irm https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.ps1 | iex
```

Install a specific version:

```powershell
$env:BADGER_VERSION = "v0.1.3"
irm https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.ps1 | iex
```

### Manual download

1. Open the [latest release](https://github.com/PVRLabs/aibadger/releases/latest).
2. Download `badger_<version>_windows_amd64.zip`.
3. Extract `badger.exe` and place it in a directory on your `PATH`.

### Build from source

```powershell
go install github.com/PVRLabs/aibadger/cmd/badger@latest
```

## Release Notes

For release publishing and artifact details, see [releasing.md](releasing.md).
