#!/usr/bin/env python3
"""Run govulncheck and fail on reachable vulnerabilities that are not accepted.

govulncheck has no ignore list. Accepted findings live in
backend/.govulncheck-ignore, one per line: `<ID> <review-by YYYY-MM-DD> <reason>`.
An entry past its review date fails the check, so an exception cannot be forgotten.

Usage: govulncheck.py <module-dir> [ignore-file]
"""
import datetime
import json
import subprocess
import sys

VERSION = "v1.1.4"


def load_ignores(path):
    out = {}
    if not path:
        return out
    try:
        lines = open(path, encoding="utf-8").read().splitlines()
    except FileNotFoundError:
        return out
    for n, line in enumerate(lines, 1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split(None, 2)
        if len(parts) < 3:
            sys.exit(f"{path}:{n}: expected '<ID> <YYYY-MM-DD> <reason>'")
        out[parts[0]] = (datetime.date.fromisoformat(parts[1]), parts[2])
    return out


def reachable(stream):
    """IDs of findings whose trace reaches a vulnerable symbol."""
    found, dec, i = {}, json.JSONDecoder(), 0
    while i < len(stream):
        while i < len(stream) and stream[i].isspace():
            i += 1
        if i >= len(stream):
            break
        obj, i = dec.raw_decode(stream, i)
        f = obj.get("finding")
        if not f:
            continue
        trace = f.get("trace") or []
        if trace and trace[0].get("function"):
            last = trace[-1]
            pos = last.get("position") or {}
            where = f"{pos.get('filename', '?')}:{pos.get('line', '?')}" if pos else last.get("function", "?")
            found.setdefault(f["osv"], where)
    return found


def main():
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    ignores = load_ignores(sys.argv[2] if len(sys.argv) > 2 else None)
    run = subprocess.run(
        ["go", "run", f"golang.org/x/vuln/cmd/govulncheck@{VERSION}", "-format", "json", "./..."],
        cwd=sys.argv[1], capture_output=True, text=True,
    )
    if run.returncode != 0:
        sys.stderr.write(run.stderr)
        sys.exit(f"govulncheck failed to run (exit {run.returncode})")
    found = reachable(run.stdout)
    today = datetime.date.today()
    failed = False
    for osv, where in sorted(found.items()):
        if osv not in ignores:
            print(f"::error::{osv} is reachable ({where}): https://pkg.go.dev/vuln/{osv}")
            failed = True
            continue
        due, reason = ignores[osv]
        if due < today:
            print(f"::error::{osv} exception expired on {due}: {reason}")
            failed = True
        else:
            print(f"accepted until {due}: {osv} ({reason})")
    for osv in sorted(set(ignores) - set(found)):
        print(f"::error::{osv} is in the ignore file but no longer reachable; remove the entry")
        failed = True
    if failed:
        sys.exit(1)
    print(f"govulncheck: {len(found)} reachable, all accepted" if found else "govulncheck: no reachable vulnerabilities")


if __name__ == "__main__":
    main()
