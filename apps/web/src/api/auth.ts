/* =============================================================================
   Auth API client — mock implementation for LAND-001 run 1.

   VITE_AUTH_MOCK:
     unset / "true"  → in-memory mock (this file)
     anything else   → stub that throws "not implemented"
                       (real HTTP client arrives in a later run)

   Only this file knows the server doesn't exist yet. Every import elsewhere
   uses `authApi` and handles `ApiRequestError` — nothing branches on mock/real.
   ============================================================================= */

import {
  ApiRequestError,
  EMAIL_PATTERN,
  PASSWORD_MIN_LENGTH,
  type AuthApi,
  type LoginRequest,
  type LoginResponse,
  type RegisterRequest,
  type RegisterResponse,
  type RequestOptions,
  type SessionInfo,
} from "./auth.types";
import {
  clearSession,
  createSession,
  findUserByEmail,
  getSession,
  sessionToCurrentUser,
  upsertUser,
} from "./mock/store";
import { performMockOAuthApprove } from "./mock/googleOAuth";

// --------------------------------------------------------------------------
// Re-export the Google OAuth helper so the consent page doesn't import
// storage internals directly.
// --------------------------------------------------------------------------
export { performMockOAuthApprove as mockOAuth };

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

/** Returns true when offline simulation is active. */
function isOffline(): boolean {
  if (sessionStorage.getItem("gs.mock.offline") === "1") return true;
  try {
    return new URLSearchParams(window.location.search).get("mock") === "offline";
  } catch {
    return false;
  }
}

/** Simulate network latency (400–700 ms). */
function delay(signal?: AbortSignal): Promise<void> {
  const ms = 400 + Math.floor(Math.random() * 301);
  return new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms);
    signal?.addEventListener("abort", () => {
      clearTimeout(timer);
      reject(new DOMException("Aborted", "AbortError"));
    });
  });
}

function throwNetwork(): never {
  throw new ApiRequestError({ kind: "network" });
}

function throwHttp(status: number, code: string, message?: string): never {
  throw new ApiRequestError({
    kind: "http",
    status,
    error: { code, message: message ?? code },
  });
}

function throwRateLimited(retryAfterSeconds: number): never {
  throw new ApiRequestError({
    kind: "http",
    status: 429,
    error: { code: "AUTH_RATE_LIMITED", message: "AUTH_RATE_LIMITED" },
    retryAfterSeconds,
  });
}

// --------------------------------------------------------------------------
// Per-email failure tracking (in-memory, intentionally not persisted)
// --------------------------------------------------------------------------

const RATE_LIMIT_THRESHOLD = 5;
const RATE_LIMIT_WINDOW_MS = 30_000;

interface FailureRecord {
  count: number;
  lockedUntil: number | null; // epoch ms
}

const failureMap = new Map<string, FailureRecord>();

function getRecord(email: string): FailureRecord {
  const key = email.toLowerCase();
  if (!failureMap.has(key)) {
    failureMap.set(key, { count: 0, lockedUntil: null });
  }
  return failureMap.get(key)!;
}

function checkRateLimit(email: string): void {
  const rec = getRecord(email);
  if (rec.lockedUntil !== null) {
    const remaining = rec.lockedUntil - Date.now();
    if (remaining > 0) {
      throwRateLimited(Math.ceil(remaining / 1000));
    }
    // Lock expired — reset
    rec.count = 0;
    rec.lockedUntil = null;
  }
}

function recordFailure(email: string): void {
  const rec = getRecord(email);
  rec.count++;
  if (rec.count >= RATE_LIMIT_THRESHOLD) {
    rec.lockedUntil = Date.now() + RATE_LIMIT_WINDOW_MS;
  }
}

function resetFailures(email: string): void {
  failureMap.delete(email.toLowerCase());
}

// --------------------------------------------------------------------------
// Mock implementation
// --------------------------------------------------------------------------

const mockImpl: AuthApi = {
  async register(req: RegisterRequest, opts?: RequestOptions): Promise<RegisterResponse> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    // Client-side validation check (belt-and-suspenders; landing page validates too)
    if (!EMAIL_PATTERN.test(req.email) || req.password.length < PASSWORD_MIN_LENGTH) {
      throwHttp(400, "VALIDATION_FAILED");
    }

    // Create user only if new; always return the same 202 body (FR-08)
    upsertUser(req.email, req.password);

    return { status: "pending_verification" };
  },

  async login(req: LoginRequest, opts?: RequestOptions): Promise<LoginResponse> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    const email = req.email.toLowerCase();

    // Check rate-limit before doing anything else
    checkRateLimit(email);

    const user = findUserByEmail(email);
    if (!user || user.password !== req.password) {
      recordFailure(email);
      // Re-check — may have just crossed the threshold
      const rec = getRecord(email);
      if (rec.lockedUntil !== null) {
        throwRateLimited(RATE_LIMIT_WINDOW_MS / 1000);
      }
      throwHttp(401, "AUTH_INVALID_CREDENTIALS");
    }

    resetFailures(email);
    const session = createSession(user.id);
    return { csrf_token: session.csrfToken };
  },

  async logout(csrfToken: string, opts?: RequestOptions): Promise<void> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    // Validate the token matches the active session
    const session = getSession();
    if (!session || session.csrfToken !== csrfToken) {
      throwHttp(401, "AUTH_INVALID_TOKEN");
    }

    clearSession();
  },

  async me(opts?: RequestOptions): Promise<SessionInfo | null> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    const session = getSession();
    if (!session) return null;

    const user = sessionToCurrentUser(session);
    if (!user) return null; // session references a deleted userId
    return { user, csrf_token: session.csrfToken };
  },

  googleAuthorizeUrl(): string {
    return "/__mock/oauth/google";
  },
};

// --------------------------------------------------------------------------
// Stub for when mock is explicitly disabled
// --------------------------------------------------------------------------

const notImplemented = (): never => {
  throw new Error("auth not implemented: set VITE_AUTH_MOCK=true or leave it unset");
};

const stubImpl: AuthApi = {
  register: notImplemented,
  login: notImplemented,
  logout: notImplemented,
  me: notImplemented,
  googleAuthorizeUrl: notImplemented,
};

// --------------------------------------------------------------------------
// Public export
// --------------------------------------------------------------------------

const useMock =
  !import.meta.env.VITE_AUTH_MOCK || import.meta.env.VITE_AUTH_MOCK === "true";

export const authApi: AuthApi = useMock ? mockImpl : stubImpl;
