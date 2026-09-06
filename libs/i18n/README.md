# Server-side i18n

**This is not the frontend's catalog.** There are two, deliberately, in separate places:

| | This directory (`libs/i18n/`) | `apps/web/src/i18n/` |
|---|---|---|
| Rendered by | The server, per request | The browser |
| Locale source | The request's `Accept-Language` header | The browser's own detection |
| Used for | The `message` in API error envelopes, emails, webhooks, and any non-browser client | Everything the SPA displays itself |
| Keyed by | Error code (`AUTH_INVALID_CREDENTIALS`) | Feature-namespaced keys (`landing.auth.error_credentials`) |

They are **not** copies of each other and are not kept in sync by content. The same failure
legitimately reads differently from each: the SPA knows it is inside a login modal and can
be specific, while the server's message must make sense with no knowledge of where it will
be displayed. Trying to share one catalog between them produces strings that are wrong in
both places.

## What must never happen here

**Audit records are never written in these languages.** Everything persisted to the audit
log is en-US, always, regardless of the request's `Accept-Language` — see
`docs/audit-and-errors.md` §3. A per-actor-locale audit log cannot be searched or
aggregated: "login failed" and "đăng nhập thất bại" would be two different things to every
query an investigator writes.

The request locale affects **the response only**. It must never reach storage.

## Structure

`locales/<locale>.json`, keyed by error code:

```json
{
  "errors": {
    "AUTH_INVALID_CREDENTIALS": "Email or password is incorrect."
  }
}
```

The key **is** the error code — there is no separate message-key indirection, so a message
key cannot dangle or drift from the code it serves.

`en-US.json` is the source of truth. Every other locale must have an entry for every code
in it; `python3 scripts/gen_audit_model.py --check` fails otherwise, and falls back to
en-US at runtime rather than rendering an empty string.
