/* =============================================================================
   Mock Google OAuth helper — creates/finds the Google stand-in user and
   writes a session.

   Exported from src/api/auth.ts as `mockOAuth` so the consent page never
   touches storage keys directly.
   ============================================================================= */

import { createSession, upsertUser } from "./store";
import type { MockSession } from "./store";

const GOOGLE_USER_EMAIL = "google.user@example.com";
const GOOGLE_USER_PASSWORD = "__oauth__";

/**
 * Creates (or finds) the Google stand-in user and opens a session.
 * Returns the new session so the shell can call markAuthenticated.
 */
export function performMockOAuthApprove(): MockSession {
  const user = upsertUser(GOOGLE_USER_EMAIL, GOOGLE_USER_PASSWORD, "user");
  return createSession(user.id);
}
