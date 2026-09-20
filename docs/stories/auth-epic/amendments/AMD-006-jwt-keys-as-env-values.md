# Amendment request: JWT keys as env-var values, not file paths

**Raised by:** the instructor, at the repo owner's explicit request
**Date:** 2026-09-19
**Blocks:** nothing — additive/simplifying change to an already-merged mechanism

## What the contract currently said

`contract.md` §5 specified `libs/authmw.LoadKeySet(privateKeyPath, pubKeyDir)`:
`AUTH_JWT_PRIVATE_KEY_PATH` pointing at a PEM file, `AUTH_JWT_PUBLIC_KEYS_PATH` pointing at a
directory of `<kid>.pub` files. `be-wiring`'s brief asked for `scripts/dev/gen-jwt-keys.sh` to
generate that file/directory pair into a git-ignored `.local/jwt/`, and `be-wiring` built exactly
that — a working, tested implementation, merged in `4ef9d37`.

## What's wrong with it

Two things, one a UX complaint and one a correctness gap it exposed:

1. **The repo owner asked for something simpler**: no separate command-line tool, no directory
   convention to remember — generate a key (a website works; `openssl` locally is the same shape
   of action) and paste it into `.env`, the same way every other secret in this repo already
   works (`POSTGRES_PASSWORD`, `AUTH_GOOGLE_CLIENT_SECRET`, etc.).
2. **`docs/deployment-aws.md` §4 already specifies env-var delivery for this exact key**, and did
   so before this run started: "`JWT_PRIVATE_KEY` / `JWT_PUBLIC_KEY`... sourced from SSM Parameter
   Store SecureString and injected at deploy time" — SSM-to-Lambda injection is an environment
   variable, not a file mount. The file-path design `be-wiring` built was never going to match how
   this actually deploys; it was local-dev-only tooling invented without that document in view
   (its own brief said "read `docs/deployment-aws.md` — you should not need to," which held for
   everything else in that document but not for this one specific variable).

Point 2 means this isn't purely a convenience change — the file-path version was the actual
deviation from what was already decided, not the other way around.

## What changed

- `libs/authmw.LoadKeySet(path, dir)` → `libs/authmw.LoadKeySetFromPEM(privatePEM string,
  previousPublicPEMs ...string)`. The public half is derived from the private key; the `kid` is
  derived from the SHA-256 fingerprint of the public key's DER encoding — nothing to generate or
  name separately for the common case for one active key.
- `AUTH_JWT_PRIVATE_KEY_PATH` / `AUTH_JWT_PUBLIC_KEYS_PATH` → `AUTH_JWT_PRIVATE_KEY` (the PEM
  value) / `AUTH_JWT_PREVIOUS_PUBLIC_KEYS` (optional, for the SEC-02 rotation window — blank-line
  separated PEM blocks, only needed while actively rotating). Both accept literal `\n` in place of
  real newlines, for contexts that can't hold a multi-line value; verified empirically that Docker
  Compose's own `.env` parser *does* support a real multi-line quoted value (`docker compose
  config` round-trips it correctly), so the literal-`\n` form is a fallback, not the documented
  primary path.
- `scripts/dev/gen-jwt-keys.sh` (wrote `signing.key` + `public/<kid>.pub` to `.local/jwt/`)
  deleted. Nothing replaces it as a file-writing tool — `openssl genpkey -algorithm RSA -pkeyopt
  rsa_keygen_bits:2048` printed to a terminal (or a VS Code task doing the same) is the whole
  "tool."
- `docker-compose.yml`'s `server` service: removed the `.local/jwt` bind mount, added
  `AUTH_JWT_PRIVATE_KEY` / `AUTH_JWT_PREVIOUS_PUBLIC_KEYS` to its passed-through environment.
- `.env.example`, `.gitignore`: updated to match — no more `.local/` entry (nothing writes there
  anymore).
- `.vscode/`: added (out of scope for the auth epic's own allowlists — general repo tooling) with
  Go debug configs; the server config loads secrets via `envFile: .env` rather than ever holding
  one in a committed file.

## Who else this affects

- `be-wiring`'s own report (`docs/stories/auth-epic/reports/be-wiring.md`) still describes the
  file-based scheme it actually built at the time — left as the historical record of that slice's
  execution, not corrected in place. This amendment is what supersedes it.
- No other slice reads `AUTH_JWT_PRIVATE_KEY_PATH`/`_PUBLIC_KEYS_PATH` or calls `LoadKeySet`
  directly — `main.go` was the only caller, already updated.
- `SessionHandler.Register` vs `RegisterPublic`/`RegisterProtected` in contract §5's Go-seams
  listing was also stale (AMD-005 changed the actual signatures but the contract snippet was never
  synced) — fixed in the same pass as this amendment since it's the same section.

## Decision

**Accepted, implemented directly** (same pattern as AMD-002/AMD-005: small, mechanical, no design
ambiguity once the deployment doc's existing env-var design was checked against). `contract.md`
§5 updated in place — this is the amendment record for that change, not a proposal awaiting a
separate decision.

**Verified:** `libs/authmw` gained direct unit tests for `LoadKeySetFromPEM` (round-trip
mint/verify, kid stability, the two-key rotation window, literal-`\n` acceptance, and three error
cases); `config` gained tests for the new env vars including the blank-line-split and
literal-`\n` behavior. Full `go build`/`vet`/`test` green. The multi-line-`.env` claim was checked
empirically with `docker compose config`, not assumed.

**Notified:** recorded here; no in-flight slice was consuming the old shape.
