#!/usr/bin/env python3
"""Fail if any locale catalog has diverged from its source-of-truth locale.

There are TWO independent catalog sets, in separate directories on purpose:

  apps/web/src/i18n/locales/   the SPA's own strings, feature-namespaced keys
  libs/i18n/locales/           the SERVER's rendered messages, keyed by error code

They are not copies of each other and their contents are not compared — the same
failure legitimately reads differently from each, because the SPA knows which
screen it is on and the server does not. What IS checked, per set, is that every
non-source locale covers every key its source locale defines.

Why this has to be mechanical: a missing key falls back silently to the source
language, so the defect ships looking like a working feature and is only found by
someone reading in the other language.

Exit 0 = all sets in parity, 1 = not.
"""
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

CATALOG_SETS = [
    ("frontend", ROOT / "apps/web/src/i18n/locales", "en-US.json", ["vi-VN.json"]),
    ("server", ROOT / "libs/i18n/locales", "en-US.json", ["vi-VN.json"]),
]


def flatten(node, prefix=""):
    keys = set()
    for key, value in node.items():
        path = f"{prefix}.{key}" if prefix else key
        keys |= flatten(value, path) if isinstance(value, dict) else {path}
    return keys


def load(path):
    if not path.exists():
        sys.exit(f"FAIL: catalog not found: {path}")
    return flatten(json.loads(path.read_text(encoding="utf-8")))


def check_set(label, directory, source, targets):
    if not directory.is_dir():
        print(f"SKIP: {label} — {directory} does not exist")
        return False
    source_keys = load(directory / source)
    failed = False
    for target in targets:
        target_keys = load(directory / target)
        missing = sorted(source_keys - target_keys)
        extra = sorted(target_keys - source_keys)
        if missing:
            failed = True
            print(f"FAIL: [{label}] {target} is missing {len(missing)} key(s) from {source}:")
            for key in missing:
                print(f"  - {key}")
        if extra:
            # Usually a rename in the source that wasn't carried across, which
            # leaves the target with a key nothing will ever read.
            failed = True
            print(f"FAIL: [{label}] {target} has {len(extra)} key(s) absent from {source}:")
            for key in extra:
                print(f"  + {key}")
        if not missing and not extra:
            print(f"OK: [{label}] {target} matches {source} ({len(source_keys)} keys)")
    return failed


def main():
    return 1 if any(check_set(*s) for s in CATALOG_SETS) else 0


if __name__ == "__main__":
    sys.exit(main())
