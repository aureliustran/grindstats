/* =============================================================================
   Error code → the SPA's own i18n key
   =============================================================================

   The server sends `{ error: { code, message } }`. The `message` is already
   localized server-side from the request's Accept-Language header — but the SPA
   normally renders its OWN string instead, because it knows context the server
   doesn't: which screen, which form, what the user was trying to do.

   **Default: render the SPA's string for the code.**
   **Exception: display the server's `message` verbatim only where a user story
   explicitly says to** — typically for errors the SPA can't anticipate, or where
   the server has detail the client lacks. `renderServerMessage` below is the one
   sanctioned way to do that, so those places are greppable.

   This map is deliberately hand-maintained rather than generated: which key an
   error maps to is a product decision, not a mechanical one. It is typed as an
   exhaustive Record<ErrorCode, string>, so adding a code to the model breaks the
   SPA's type-check until someone decides what it should say here. That failure
   is the point — a silently unhandled error code renders nothing to the user.
   ============================================================================= */

import type { ErrorCode } from "../api/generated/audit";

/** Every declared error code, mapped to a key in this app's own catalogs. */
export const ERROR_MESSAGE_KEYS: Record<ErrorCode, string> = {
  AUTH_INVALID_CREDENTIALS: "landing.auth.error_credentials",
  AUTH_INVALID_TOKEN: "common.auth.error_invalid_token",
  AUTH_SESSION_EXPIRED: "common.auth.error_session_expired",
  AUTH_FORBIDDEN: "common.auth.error_forbidden",
  AUTH_CSRF_FAILED: "common.auth.error_csrf",
  AUTH_ACCOUNT_SUSPENDED: "common.auth.error_suspended",
  AUTH_RATE_LIMITED: "landing.auth.error_rate_limited",
  VALIDATION_FAILED: "common.error_validation",
  SERVICE_UNAVAILABLE: "common.error_service_unavailable",
  INTERNAL_ERROR: "common.error_internal",
};

export interface ApiError {
  code: ErrorCode | string;
  /** Server-rendered, already localized via Accept-Language. */
  message?: string;
}

/**
 * The SPA's own text for an API error. Use this by default.
 *
 * Falls back to the server's message for a code this build doesn't know — which
 * happens legitimately when the server is ahead of the deployed frontend. That
 * fallback is why the server renders a message at all: an unrecognized code
 * still shows the user something true rather than a blank or a raw identifier.
 */
export function errorMessageKey(error: ApiError): string | null {
  const key = ERROR_MESSAGE_KEYS[error.code as ErrorCode];
  return key ?? null;
}

/**
 * Display the SERVER's localized message rather than the SPA's own.
 *
 * Only for places a story explicitly permits. Call this instead of reaching into
 * `error.message` directly, so every such place is findable with one grep when
 * the policy is revisited.
 */
export function renderServerMessage(error: ApiError): string | undefined {
  return error.message;
}
