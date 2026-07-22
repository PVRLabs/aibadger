#!/bin/sh
set -eu

repo="${BADGER_REPO:-PVRLabs/aibadger}"
install_dir="${BADGER_INSTALL_DIR:-}"
version="${BADGER_VERSION:-}"
binary_name="badger"

fail() {
  echo "badger installer: $*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

need curl
need tar
need mktemp
need uname
need awk

path_has_dir() {
  case ":${PATH:-}:" in
    *:"$1":*) return 0 ;;
    *) return 1 ;;
  esac
}

case "$(uname -s)" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *) fail "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *) fail "unsupported architecture: $(uname -m)" ;;
esac

if [ -z "$version" ]; then
  latest_url="$(curl -fsSLo /dev/null -w '%{url_effective}' "https://github.com/${repo}/releases/latest")"
  version="${latest_url##*/}"
fi

case "$version" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) fail "invalid version: ${version}. Expected a tag like v0.2.0" ;;
esac

version_number="${version#v}"
archive_name="${binary_name}_${version_number}_${os}_${arch}.tar.gz"
base_url="https://github.com/${repo}/releases/download/${version}"
archive_url="${base_url}/${archive_name}"
checksum_url="${archive_url}.sha256"

if [ -z "$install_dir" ]; then
  [ -n "${HOME:-}" ] || fail "HOME is not set; set BADGER_INSTALL_DIR to choose an install directory"

  if mkdir -p "${HOME}/.local/bin" 2>/dev/null; then
    install_dir="${HOME}/.local/bin"
  elif mkdir -p "${HOME}/bin" 2>/dev/null; then
    install_dir="${HOME}/bin"
  else
    fail "could not create ${HOME}/.local/bin or ${HOME}/bin; set BADGER_INSTALL_DIR"
  fi
else
  mkdir -p "$install_dir" || fail "could not create install directory: ${install_dir}"
fi

[ -d "$install_dir" ] || fail "install target is not a directory: ${install_dir}"
[ -w "$install_dir" ] || fail "install target is not writable: ${install_dir}"
install_dir="$(cd "$install_dir" && pwd -P)"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

archive_path="${tmp_dir}/${archive_name}"
checksum_path="${archive_path}.sha256"

echo "Downloading AIBadger ${version} for ${os}/${arch}..."
curl -fsSL "$archive_url" -o "$archive_path"
curl -fsSL "$checksum_url" -o "$checksum_path"

expected_hash="$(awk '{print $1; exit}' "$checksum_path")"
[ -n "$expected_hash" ] || fail "checksum file is empty: ${checksum_url}"

if command -v sha256sum >/dev/null 2>&1; then
  actual_hash="$(sha256sum "$archive_path" | awk '{print $1; exit}')"
elif command -v shasum >/dev/null 2>&1; then
  actual_hash="$(shasum -a 256 "$archive_path" | awk '{print $1; exit}')"
else
  fail "missing required command: sha256sum or shasum"
fi

[ "$actual_hash" = "$expected_hash" ] || fail "checksum mismatch for ${archive_name}"

tar -xzf "$archive_path" -C "$tmp_dir" "$binary_name"
[ -f "${tmp_dir}/${binary_name}" ] || fail "archive did not contain ${binary_name}"

cp "${tmp_dir}/${binary_name}" "${install_dir}/${binary_name}.new"
chmod 0755 "${install_dir}/${binary_name}.new"
mv -f "${install_dir}/${binary_name}.new" "${install_dir}/${binary_name}"

symlink_created=""
rc_updated=""
rc_file=""

try_symlink() {
  candidate="$1"

  [ "$candidate" != "$install_dir" ] || return 0
  path_has_dir "$candidate" || return 0
  [ -d "$candidate" ] && [ -w "$candidate" ] || return 0

  if ln -sf "${install_dir}/${binary_name}" "${candidate}/${binary_name}"; then
    symlink_created="$candidate"
    echo "Symlinked ${candidate}/${binary_name} -> ${install_dir}/${binary_name}"
  fi
}

update_rc_file() {
  candidate_rc="$1"
  candidate_dir="${candidate_rc%/*}"
  rc_tmp="${tmp_dir}/badger-rc"

  if [ ! -e "$candidate_rc" ]; then
    mkdir -p "$candidate_dir" 2>/dev/null || return 0
    [ -w "$candidate_dir" ] || return 0
  elif [ ! -w "$candidate_rc" ]; then
    return 0
  fi

  if [ -f "$candidate_rc" ]; then
    awk '
      /^# >>> badger installer >>>$/ { skipping = 1; next }
      /^# <<< badger installer <<<$/ { skipping = 0; next }
      !skipping { print }
    ' "$candidate_rc" > "$rc_tmp" || return 0
  else
    : > "$rc_tmp" || return 0
  fi

  if [ "$user_shell" = "fish" ]; then
    path_line="fish_add_path $path_entry"
  else
    path_line="export PATH=\"${path_entry}:\$PATH\""
  fi

  {
    printf '\n# >>> badger installer >>>\n'
    printf '%s\n' "$path_line"
    printf '# <<< badger installer <<<\n'
  } >> "$rc_tmp" || return 0

  if [ -e "$candidate_rc" ]; then
    cat "$rc_tmp" > "$candidate_rc" || return 0
  else
    mv -f "$rc_tmp" "$candidate_rc" || return 0
  fi

  rc_file="$candidate_rc"
  rc_updated=1
  echo "Updated PATH in ${candidate_rc}"
}

if ! path_has_dir "$install_dir"; then
  if [ -n "${HOME:-}" ]; then
    try_symlink "${HOME}/.local/bin"
    [ -n "$symlink_created" ] || try_symlink "${HOME}/bin"
  fi
  [ -n "$symlink_created" ] || try_symlink "/usr/local/bin"

  if [ -n "${HOME:-}" ]; then
    user_shell="${SHELL:-}"
    user_shell="${user_shell##*/}"
    path_entry="$install_dir"
    case "$install_dir" in
      "${HOME}/.local/bin") path_entry='${HOME}/.local/bin' ;;
      "${HOME}/bin") path_entry='${HOME}/bin' ;;
    esac

    case "$user_shell" in
      bash) update_rc_file "${HOME}/.bashrc" ;;
      zsh) update_rc_file "${HOME}/.zshrc" ;;
      fish) update_rc_file "${HOME}/.config/fish/config.fish" ;;
    esac

    if [ "$os" = "Darwin" ] && [ "$user_shell" = "bash" ] && [ -n "$rc_updated" ] && [ -f "${HOME}/.bash_profile" ]; then
      if ! awk '/bashrc/ { found = 1 } END { exit !found }' "${HOME}/.bash_profile"; then
        {
          printf '\n# >>> badger installer bash profile >>>\n'
          printf '[[ -r ~/.bashrc ]] && source ~/.bashrc\n'
          printf '# <<< badger installer bash profile <<<\n'
        } >> "${HOME}/.bash_profile" 2>/dev/null || true
      fi
    fi
  fi
fi

echo "Installed ${binary_name} to ${install_dir}/${binary_name}"

if path_has_dir "$install_dir" || [ -n "$symlink_created" ]; then
  echo "Run '${binary_name}' to get started."
elif [ -n "$rc_updated" ]; then
  echo "Restart your terminal (or source ${rc_file}), then run '${binary_name}'."
else
  echo "Add ${install_dir} to your PATH to run ${binary_name} from any directory:"
  echo "  export PATH=\"${install_dir}:\$PATH\""
fi

"${install_dir}/${binary_name}" --version
