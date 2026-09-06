#!/usr/bin/env python3
"""Validate libs/auditmodel/model.yaml and generate its Go and TypeScript consumers.

The model file is the single source of truth for enums, audit events and error
codes. This script is what makes that claim true rather than aspirational: it
fails on the mistakes that would otherwise be caught only in review (or not at
all), then emits the code both sides import.

Usage:
    python3 scripts/gen_audit_model.py            # validate + generate
    python3 scripts/gen_audit_model.py --check    # validate only; no writes

--check is the CI mode: it also fails if the generated files are out of date,
which is what stops someone editing generated.go by hand and having it silently
reverted on the next run.
"""
import argparse
import re
import sys
from pathlib import Path

try:
    import yaml
except ImportError:
    sys.exit("PyYAML required: pip install pyyaml --break-system-packages")

ROOT = Path(__file__).resolve().parent.parent
MODEL = ROOT / "libs/auditmodel/model.yaml"
GO_OUT = ROOT / "libs/auditmodel/generated.go"
TS_OUT = ROOT / "apps/web/src/api/generated/audit.ts"
SERVER_LOCALES = ROOT / "libs/i18n/locales"
SERVER_SOURCE_LOCALE = "en-US"

# Field names that would mean a secret is being written to an append-only,
# 90-day-retained log. Log the handle (jti), never the token itself (NFR-07).
FORBIDDEN_FIELD_PATTERNS = [
    r"password", r"passwd", r"secret", r"\bhash\b", r"credential",
    r"^token$", r"_token$", r"^access_token", r"^refresh_token", r"api_key",
]

EVENT_NAME_RE = re.compile(r"^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$")
CODE_NAME_RE = re.compile(r"^[A-Z][A-Z0-9_]*$")
PLACEHOLDER_RE = re.compile(r"\{([a-z_][a-z0-9_]*)\}")


def server_catalogs():
    """{locale: {code: message}} from libs/i18n/locales, or None if absent.

    These are the SERVER's rendering catalogs, keyed by error code — not the
    frontend's, which live in apps/web/src/i18n and are the SPA's own business.
    """
    import json

    if not SERVER_LOCALES.is_dir():
        return None
    out = {}
    for path in sorted(SERVER_LOCALES.glob("*.json")):
        data = json.loads(path.read_text(encoding="utf-8"))
        out[path.stem] = data.get("errors", {})
    return out or None


def fail(errors):
    for e in errors:
        print(f"FAIL: {e}", file=sys.stderr)
    sys.exit(1)


def validate(model):
    errors = []
    catalogs = server_catalogs()
    enums = model.get("enums", {})
    codes = model.get("error_codes", {})
    events = model.get("events", {})

    for name in enums:
        if not re.match(r"^[A-Z][A-Za-z0-9]*$", name):
            errors.append(f"enum '{name}': name must be PascalCase")
        vals = enums[name].get("values", {})
        if not vals:
            errors.append(f"enum '{name}': has no values")
        for v in vals:
            if not re.match(r"^[a-z][a-z0-9_]*$", str(v)):
                errors.append(
                    f"enum '{name}' value '{v}': must be lower_snake_case. "
                    "Enum values are permanent storage identifiers, not display text."
                )

    for name, spec in codes.items():
        if not CODE_NAME_RE.match(name):
            errors.append(f"error code '{name}': must be SCREAMING_SNAKE_CASE")
        if "http_status" not in spec:
            errors.append(f"error code '{name}': missing http_status")
        if "message" in spec or "message_key" in spec:
            errors.append(
                f"error code '{name}': message text does not belong in the model. The "
                "server renders it from libs/i18n/locales/<locale>.json, keyed by the "
                "code itself."
            )
        if catalogs is not None:
            source = catalogs.get(SERVER_SOURCE_LOCALE, {})
            if name not in source:
                errors.append(
                    f"error code '{name}': no entry in libs/i18n/locales/"
                    f"{SERVER_SOURCE_LOCALE}.json. The server cannot render a message "
                    "for it, so callers would receive an empty one."
                )
            for locale, entries in catalogs.items():
                if locale != SERVER_SOURCE_LOCALE and name in source and name not in entries:
                    errors.append(
                        f"error code '{name}': missing from libs/i18n/locales/{locale}.json"
                    )

    for name, spec in events.items():
        if not EVENT_NAME_RE.match(name):
            errors.append(f"event '{name}': must be dotted.lower_snake (e.g. auth.login.failed)")
        for required in ("description", "actor", "outcome", "severity", "message"):
            if required not in spec:
                errors.append(f"event '{name}': missing '{required}'")

        for key, enum_name in (("actor", "ActorType"), ("outcome", "Outcome"), ("severity", "Severity")):
            val = spec.get(key)
            if val is not None:
                allowed = enums.get(enum_name, {}).get("values", {})
                if val not in allowed:
                    errors.append(f"event '{name}': {key}='{val}' is not a value of {enum_name}")

        fields = spec.get("fields", {}) or {}
        for fname, fspec in fields.items():
            for pattern in FORBIDDEN_FIELD_PATTERNS:
                if re.search(pattern, fname):
                    errors.append(
                        f"event '{name}' field '{fname}': looks like a secret. Audit records "
                        "are append-only and retained 90+ days — log the handle (jti), never "
                        "the credential (NFR-07, SEC-01)."
                    )
            if "enum" in fspec:
                if fspec["enum"] not in enums:
                    errors.append(f"event '{name}' field '{fname}': unknown enum '{fspec['enum']}'")
            elif "type" not in fspec:
                errors.append(f"event '{name}' field '{fname}': needs either 'enum' or 'type'")

        # Every {placeholder} in the message must be a declared field.
        for ph in PLACEHOLDER_RE.findall(spec.get("message", "")):
            if ph not in fields:
                errors.append(
                    f"event '{name}': message references {{{ph}}} but no such field is declared"
                )

        ec = spec.get("error_code")
        if ec is not None and ec not in codes:
            errors.append(f"event '{name}': unknown error_code '{ec}'")

    # Catalog entries for codes that no longer exist are dead weight that later
    # reads as though the code is still supported.
    if catalogs is not None:
        for locale, entries in catalogs.items():
            for key in entries:
                if key not in codes:
                    errors.append(
                        f"libs/i18n/locales/{locale}.json: entry '{key}' is not a declared "
                        "error code"
                    )

    return errors


def go_ident(s):
    return "".join(p.capitalize() for p in re.split(r"[._]", s))


def gen_go(model):
    L = [
        "// Code generated by scripts/gen_audit_model.py. DO NOT EDIT.",
        "// Source: libs/auditmodel/model.yaml",
        "//",
        "// Hand edits are reverted on the next generation run. Change the model file.",
        "",
        "package auditmodel",
        "",
    ]
    L.append("// --- Enums ---------------------------------------------------------------")
    L.append("")
    for name, spec in model["enums"].items():
        desc = (spec.get("description") or "").strip().replace("\n", " ")
        L.append(f"// {name}: {desc}")
        L.append(f"type {name} string")
        L.append("")
        L.append("const (")
        for v in spec.get("values", {}):
            L.append(f'\t{name}{go_ident(v)} {name} = "{v}"')
        L.append(")")
        L.append("")
        L.append(f"// All{name}s lists every valid value, for validation and iteration.")
        L.append(f"var All{name}s = []{name}{{")
        for v in spec.get("values", {}):
            L.append(f"\t{name}{go_ident(v)},")
        L.append("}")
        L.append("")
        L.append(f"func (v {name}) Valid() bool {{")
        L.append(f"\tfor _, c := range All{name}s {{")
        L.append("\t\tif v == c {")
        L.append("\t\t\treturn true")
        L.append("\t\t}")
        L.append("\t}")
        L.append("\treturn false")
        L.append("}")
        L.append("")

    L.append("// --- Error codes ---------------------------------------------------------")
    L.append("")
    L.append("type ErrorCode string")
    L.append("")
    L.append("const (")
    for name, spec in model["error_codes"].items():
        L.append(f'\tErr{go_ident(name.lower())} ErrorCode = "{name}"')
    L.append(")")
    L.append("")
    L.append("// ErrorCodeSpec carries what the API layer needs to build an error envelope.")
    L.append("//")
    L.append("// There is no message text here. The envelope's message is rendered by the")
    L.append("// server from libs/i18n/locales/<locale>.json, keyed by the code itself,")
    L.append("// using the request's Accept-Language. See libs/i18n/README.md.")
    L.append("type ErrorCodeSpec struct {")
    L.append("\tHTTPStatus int")
    L.append("}")
    L.append("")
    L.append("var ErrorCodes = map[ErrorCode]ErrorCodeSpec{")
    for name, spec in model["error_codes"].items():
        L.append(f'\tErr{go_ident(name.lower())}: {{HTTPStatus: {spec["http_status"]}}},')
    L.append("}")
    L.append("")
    L.append("// AllErrorCodes lets a catalog completeness test iterate every code.")
    L.append("var AllErrorCodes = []ErrorCode{")
    for name in model["error_codes"]:
        L.append(f"\tErr{go_ident(name.lower())},")
    L.append("}")
    L.append("")

    L.append("// --- Audit events --------------------------------------------------------")
    L.append("")
    L.append("type AuditEvent string")
    L.append("")
    L.append("const (")
    for name, spec in model["events"].items():
        desc = (spec.get("description") or "").strip().split("\n")[0]
        L.append(f"\t// {desc}")
        L.append(f'\tEvt{go_ident(name)} AuditEvent = "{name}"')
    L.append(")")
    L.append("")
    L.append("// AuditEventSpec describes one event's fixed attributes and its declared fields.")
    L.append("// RequiredFields is what an emitter must supply; emitting without them is a bug")
    L.append("// that leaves an unqueryable record in an append-only store.")
    L.append("type AuditEventSpec struct {")
    L.append("\tActor          ActorType")
    L.append("\tOutcome        Outcome")
    L.append("\tSeverity       Severity")
    L.append("\tMessage        string")
    L.append("\tRequiredFields []string")
    L.append("\tErrorCode      ErrorCode // empty when the event surfaces nothing to the caller")
    L.append("}")
    L.append("")
    L.append("var AuditEvents = map[AuditEvent]AuditEventSpec{")
    for name, spec in model["events"].items():
        fields = spec.get("fields", {}) or {}
        required = [f for f, fs in fields.items() if not fs.get("optional")]
        req = ", ".join(f'"{f}"' for f in required)
        ec = spec.get("error_code")
        ec_go = f"Err{go_ident(ec.lower())}" if ec else '""'
        msg = spec["message"].replace('"', '\\"')
        L.append(f"\tEvt{go_ident(name)}: {{")
        L.append(f"\t\tActor:          ActorType{go_ident(spec['actor'])},")
        L.append(f"\t\tOutcome:        Outcome{go_ident(spec['outcome'])},")
        L.append(f"\t\tSeverity:       Severity{go_ident(spec['severity'])},")
        L.append(f'\t\tMessage:        "{msg}",')
        L.append(f"\t\tRequiredFields: []string{{{req}}},")
        L.append(f"\t\tErrorCode:      {ec_go},")
        L.append("\t},")
    L.append("}")
    L.append("")
    return "\n".join(L)


def gen_ts(model):
    L = [
        "// Code generated by scripts/gen_audit_model.py. DO NOT EDIT.",
        "// Source: libs/auditmodel/model.yaml",
        "//",
        "// Hand edits are reverted on the next generation run. Change the model file.",
        "//",
        "// Status values and error codes shared with the backend. Importing from here",
        "// instead of retyping a literal is what keeps the two sides from drifting.",
        "//",
        "// Message TEXT is not shared — see the note at the bottom of this file.",
        "",
    ]
    for name, spec in model["enums"].items():
        desc = (spec.get("description") or "").strip().replace("\n", " ")
        vals = list(spec.get("values", {}))
        L.append(f"/** {desc} */")
        L.append(f"export type {name} =")
        L.extend([f'  | "{v}"' for v in vals])
        L[-1] += ";"
        L.append(f"export const ALL_{re.sub(r'(?<!^)(?=[A-Z])', '_', name).upper()}: readonly {name}[] = [")
        L.extend([f'  "{v}",' for v in vals])
        L.append("] as const;")
        L.append("")

    L.append("/** Stable error identifiers. Switch on these — never on the message. */")
    L.append("export type ErrorCode =")
    codes = list(model["error_codes"])
    L.extend([f'  | "{c}"' for c in codes])
    L[-1] += ";"
    L.append("")
    L.append("// No message text or i18n keys are generated here on purpose.")
    L.append("//")
    L.append("// The SPA renders its OWN strings for these codes from apps/web/src/i18n,")
    L.append("// because it knows the context (which screen, which form) and the server")
    L.append("// does not. The server also sends a rendered `message` in the envelope,")
    L.append("// localized via Accept-Language — display THAT only where a story")
    L.append("// explicitly allows it (see docs/frontend.md).")
    L.append("//")
    L.append("// Map codes to your own keys with Record<ErrorCode, string> so that adding")
    L.append("// a code here fails the SPA's type-check until it is handled:")
    L.append("// see apps/web/src/i18n/errorMessages.ts")
    L.append("")
    L.append("export const ERROR_HTTP_STATUS: Record<ErrorCode, number> = {")
    for name, spec in model["error_codes"].items():
        L.append(f"  {name}: {spec['http_status']},")
    L.append("};")
    L.append("")
    return "\n".join(L)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--check", action="store_true", help="validate only; fail if generated files are stale")
    args = ap.parse_args()

    model = yaml.safe_load(MODEL.read_text(encoding="utf-8"))
    errors = validate(model)
    if errors:
        fail(errors)

    go_src, ts_src = gen_go(model), gen_ts(model)

    if args.check:
        stale = [
            str(p.relative_to(ROOT))
            for p, src in ((GO_OUT, go_src), (TS_OUT, ts_src))
            if not p.exists() or p.read_text(encoding="utf-8") != src
        ]
        if stale:
            fail([f"generated file out of date: {s} (run scripts/gen_audit_model.py)" for s in stale])
        print(
            f"OK: model valid and generated files current "
            f"({len(model['enums'])} enums, {len(model['error_codes'])} error codes, "
            f"{len(model['events'])} events)"
        )
        return

    GO_OUT.parent.mkdir(parents=True, exist_ok=True)
    TS_OUT.parent.mkdir(parents=True, exist_ok=True)
    GO_OUT.write_text(go_src, encoding="utf-8")
    TS_OUT.write_text(ts_src, encoding="utf-8")
    print(f"OK: wrote {GO_OUT.relative_to(ROOT)} and {TS_OUT.relative_to(ROOT)}")
    print(
        f"     {len(model['enums'])} enums, {len(model['error_codes'])} error codes, "
        f"{len(model['events'])} events"
    )


if __name__ == "__main__":
    main()
