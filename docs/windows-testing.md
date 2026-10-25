# Windows CI checks

The regular `ci` workflow builds the current revision on `windows-latest` and
runs CLI/map smoke checks. It also runs `scripts/windows-smoke.ps1` under Windows
PowerShell 5.1 and PowerShell 7, covering:

- CLI help and agreement between `version` and `--version`.
- Windows diagnosis, plus numeric version reporting for Maven, Gradle and npm
  batch launchers when installed. Missing batch launchers are explicitly skipped.
  Native Windows unit tests also exercise synthetic `.cmd`/`.bat` version probes
  in paths containing spaces, Unicode, and shell metacharacters, independently
  of the runner's installed tools. CI also opts into an installed-npm probe
  using the same deadline policy, temporary project directory, environment
  override, and parser as diagnose. Failures report only execution categories
  (timeout, exit status, launch/I/O error, or inherited-pipe delay) and counts
  of numeric/warning/other output lines; raw output and paths are suppressed.
- API code/design prompts with Unicode UTF-8 input, absolute and relative Windows
  paths, nested directories, and spaces in project/input paths.

Fixtures are disposable and removed after the checks. Command output and prompt
contents are not logged. These checks do not build fixture projects or install
third-party development tools. The existing CI build still requires Go.

Windows npm probes allow ten seconds by default for cold launcher startup;
other probes retain the three-second default. An explicit diagnose API timeout
overrides either default. The installed-npm test uses the product deadline policy
and still fails if npm cannot return a valid version within that bound.

## Manual installer action

In GitHub Actions, select **Windows installer smoke**, choose the workflow ref,
and enter an exact published release tag such as `v0.0.0` (replace with a real
release). No separate Windows host is needed. The job:

1. Runs the selected ref's `install.ps1` unchanged on a disposable GitHub-hosted
   Windows runner, installing the pinned release into a temporary path with spaces.
2. Requires the published Windows amd64 archive SHA-256 checksum. Its download
   adapter checks the actual downloaded archive before returning control to the
   installer's extraction step. Missing, malformed or mismatched checksums fail
   the job. This checks publisher integrity, not an independent signature.
3. Checks installed version, persistent user PATH addition, and executable
   discovery through the installer's session PATH.
4. Runs the shared CLI smoke checks against the installed executable in both
   PowerShell versions. No build toolchain is needed for this job.

The runner-only installer harness intentionally changes user PATH, restores its
previous value in `finally`, and cleans temporary installation files after the
second shell finishes. It refuses to run outside a GitHub-hosted Windows runner.
The action is opt-in, downloads the selected release, and has a five-minute job
limit. It tests a custom installation directory, not the default directory or
executable discovery in a newly logged-in session. This is a fresh-install test:
it does not attempt an upgrade while Badger is running or exercise replacement
of a running executable and handling of locked backup files. Those upgrade
behaviors remain unvalidated by this workflow.

These jobs do not validate interactive TUI input/rendering, Badger's clipboard
writer, Explorer selection, or VS Code discovery. Passing CI establishes the
listed automated behavior on the hosted Windows Server image; it does not
establish desktop acceptance for those workflows.
