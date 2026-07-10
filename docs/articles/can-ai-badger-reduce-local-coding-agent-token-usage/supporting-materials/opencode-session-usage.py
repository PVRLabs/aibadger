#!/usr/bin/env python3
"""
OpenCode session usage utility.

Usage:
  ./opencode-session-usage.py
      Print the latest 10 OpenCode sessions (non-interactive).

  ./opencode-session-usage.py <session_id>
      Export one OpenCode session and print usage stats.

Requires the `opencode` binary in PATH.
"""

import json
import os
import shutil
import subprocess
import sys

LATEST_SESSION_LIMIT = 10


def main():
    if len(sys.argv) > 2:
        print("Usage: opencode-session-usage.py [<session_id>]", file=sys.stderr)
        sys.exit(1)

    if len(sys.argv) == 2 and sys.argv[1] in ("--help", "-h"):
        print(
            f"Usage:\n"
            f"  {sys.argv[0]}\n"
            f"      Print the latest {LATEST_SESSION_LIMIT} OpenCode sessions.\n"
            f"\n"
            f"  {sys.argv[0]} <session_id>\n"
            f"      Print usage stats for one OpenCode session.\n"
        )
        return

    if not _find_opencode():
        print("Error: opencode is required and was not found in PATH.", file=sys.stderr)
        sys.exit(1)

    if len(sys.argv) == 1:
        print_latest_sessions()
    else:
        print_session_usage(sys.argv[1])


def _find_opencode():
    return shutil.which("opencode") is not None


def print_latest_sessions():
    env = os.environ | {"PAGER": "cat", "NO_COLOR": "1"}
    result = subprocess.run(
        ["opencode", "session", "list"],
        capture_output=True, text=True, env=env
    )
    if result.returncode != 0:
        print("Error: opencode session list failed", file=sys.stderr)
        sys.exit(1)

    lines = result.stdout.splitlines()
    print(f"Latest {LATEST_SESSION_LIMIT} OpenCode sessions:\n")
    for line in lines[:LATEST_SESSION_LIMIT]:
        print(line)


def print_session_usage(sid):
    result = subprocess.run(
        ["opencode", "export", sid],
        capture_output=True, text=True
    )
    if result.returncode != 0:
        print(f"Error: opencode export {sid} failed", file=sys.stderr)
        sys.exit(1)

    raw = result.stdout

    # opencode export may print a human-readable line before the JSON
    brace = raw.find("{")
    if brace < 0:
        print(f"Error: no JSON found in export for session {sid}", file=sys.stderr)
        sys.exit(1)
    raw = raw[brace:]

    # opencode export truncates at 64KB, producing broken JSON.
    # The .info object is always before .messages and always complete.
    info = _extract_info(raw, sid)
    if not info:
        print(f"Error: could not extract session info from export for {sid}", file=sys.stderr)
        sys.exit(1)

    _print_info(info)


def _extract_info(raw, sid):
    ki = raw.find('"info"')
    if ki < 0:
        return {}
    brace = raw.find("{", ki + 6)
    if brace < 0:
        return {}

    try:
        obj, _ = json.JSONDecoder().raw_decode(raw[brace:])
        return obj
    except json.JSONDecodeError:
        print(f"Error: could not extract session info from export for {sid}", file=sys.stderr)
        sys.exit(1)


def _print_info(info):
    i = info.get("tokens", {})
    s = info.get("summary", {})
    t = info.get("time", {})

    inp = i.get("input", 0) or 0
    out = i.get("output", 0) or 0
    rea = i.get("reasoning", 0) or 0
    cache_r = i.get("cache", {}).get("read", 0) or 0
    cache_w = i.get("cache", {}).get("write", 0) or 0

    files = s.get("files", 0) or 0
    adds = s.get("additions", 0) or 0
    dels = s.get("deletions", 0) or 0

    active = inp + out + rea
    cache_total = cache_r + cache_w
    total = active + cache_total

    created = t.get("created")
    updated = t.get("updated")
    duration_ms = _format_duration(created, updated)
    tokens_per_file = _format_division(total, files)
    tokens_per_addline = _format_division(total, adds)
    active_per_file = _format_division(active, files)

    model = info.get("model", {}) or {}
    prov = model.get("providerID", "") or ""
    mid = model.get("id", "") or ""

    print(
        f"SESSION\n"
        f"  id:                 {info.get('id', '')}\n"
        f"  title:              {info.get('title', '')}\n"
        f"  directory:          {info.get('directory', '')}\n"
        f"  opencode_version:   {info.get('version', '')}\n"
        f"  agent:              {info.get('agent', '')}\n"
        f"  model:              {prov}/{mid}\n"
        f"  cost:               {info.get('cost', 0)}\n"
        f"\n"
        f"TOKENS\n"
        f"  input:              {inp}\n"
        f"  output:             {out}\n"
        f"  reasoning:          {rea}\n"
        f"  cache_read:         {cache_r}\n"
        f"  cache_write:        {cache_w}\n"
        f"  active_total:       {active}\n"
        f"  cache_total:        {cache_total}\n"
        f"  total_with_cache:   {total}\n"
        f"\n"
        f"CHANGE SUMMARY\n"
        f"  files_changed:      {files}\n"
        f"  additions:          {adds}\n"
        f"  deletions:          {dels}\n"
        f"\n"
        f"DERIVED\n"
        f"  duration_ms:        {duration_ms}\n"
        f"  tokens_per_file:    {tokens_per_file}\n"
        f"  tokens_per_addline: {tokens_per_addline}\n"
        f"  active_per_file:    {active_per_file}"
    )


def _is_number(value):
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def _format_division(numerator, denominator):
    if not _is_number(denominator) or denominator <= 0:
        return "N/A"
    if not _is_number(numerator):
        return "N/A"
    return str(int(numerator) // int(denominator))


def _format_duration(created, updated):
    if not _is_number(created) or not _is_number(updated):
        return "N/A"
    if updated < created:
        return "N/A"
    return str(int(updated) - int(created))


if __name__ == "__main__":
    main()
