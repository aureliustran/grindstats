#!/usr/bin/env python3
"""Fail if the locale catalogs have diverged.

Why this exists: docs/i18n-guidelines.md makes en-US the source of truth and
requires every key to exist in vi-VN too. That rule is only real if something
checks it — a missing key otherwise falls back to English silently and ships.

Wire this into CI (roadmap Phase 12) and into your pre-commit if you like.
Exit code 0 = catalogs match, 1 = they don't.
"""
import json
import sys
from pathlib import Path

LOCALES_DIR = Path(__file__).resolve().parent.parent / "apps/web/src/i18n/locales"
SOURCE = "en-US.json"
TARGETS = ["vi-VN.json"]


def flatten(node, prefix=""):
    keys = set()
    for key, value in node.items():
        path = f"{prefix}.{key}" if prefix else key
        keys |= flatten(value, path) if isinstance(value, dict) else {path}
    return keys


def load(name):
    path = LOCALES_DIR / name
    if not path.exists():
        sys.exit(f"FAIL: catalog not found: {path}")
    return flatten(json.loads(path.read_text(encoding="utf-8")))


def main():
    source_keys = load(SOURCE)
    failed = False

    for target in TARGETS:
        target_keys = load(target)
        missing = sorted(source_keys - target_keys)
        extra = sorted(target_keys - source_keys)

        if missing:
            failed = True
            print(f"FAIL: {target} is missing {len(missing)} key(s) present in {SOURCE}:")
            for key in missing:
                print(f"  - {key}")
        if extra:
            # Not fatal on its own, but it usually means a key was renamed in
            # en-US and the rename wasn't carried across — worth failing on.
            failed = True
            print(f"FAIL: {target} has {len(extra)} key(s) absent from {SOURCE}:")
            for key in extra:
                print(f"  + {key}")
        if not missing and not extra:
            print(f"OK: {target} matches {SOURCE} ({len(source_keys)} keys)")

    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
