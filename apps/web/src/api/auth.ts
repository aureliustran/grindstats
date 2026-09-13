/* =============================================================================
   Auth API client — real HTTP implementation (default) and mock for tests/offline.
   =============================================================================

   VITE_AUTH_MOCK:
     "true"         → in-memory mock
     unset / other  → real HTTP client (apiFetch)
   ============================================================================= */

import {
  ApiRequestError,
  EMAIL_PATTERN,
  PASSWORD_MIN_LENGTH,
  type AuthApi,
  type LoginRequest,
  type LoginResponse,
  type OAuthLinkConfirmRequest,
  type PasswordResetConfirmRequest,
  type PasswordResetRequestRequest,
  type RegisterRequest,
  type RegisterResponse,
  type RequestOptions,
  type SessionInfo,
  type StatusResponse,
  type VerifyEmailRequest,
} from "./auth.types";
import { apiFetch, setCsrfToken } from "./http";
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
// Re-export the Google OAuth helper
// --------------------------------------------------------------------------
export { performMockOAuthApprove as mockOAuth };

// --------------------------------------------------------------------------
// Real HTTP implementation
// --------------------------------------------------------------------------

export const realImpl: AuthApi = {
  async register(req: RegisterRequest, opts?: RequestOptions): Promise<RegisterResponse> {
    return await apiFetch<RegisterResponse>("/api/v1/auth/register", {
      method: "POST",
      body: JSON.stringify(req),
      signal: opts?.signal,
    });
  },

  async login(req: LoginRequest, opts?: RequestOptions): Promise<LoginResponse> {
    const res = await apiFetch<LoginResponse>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify(req),
      signal: opts?.signal,
    });
    if (res?.csrf_token) {
      setCsrfToken(res.csrf_token);
    }
    return res;
  },

  async logout(csrfToken: string, opts?: RequestOptions): Promise<void> {
    try {
      await apiFetch<void>("/api/v1/auth/logout", {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken },
        signal: opts?.signal,
      });
    } finally {
      setCsrfToken(null);
    }
  },

  async me(opts?: RequestOptions): Promise<SessionInfo | null> {
    try {
      const res = await apiFetch<SessionInfo>("/api/v1/users/me", {
        method: "GET",
        signal: opts?.signal,
      });
      if (res?.csrf_token) {
        setCsrfToken(res.csrf_token);
      }
      return res;
    } catch (err) {
      if (err instanceof ApiRequestError && err.failure.kind === "http" && err.failure.status === 401) {
        setCsrfToken(null);
        return null;
      }
      throw err;
    }
  },

  googleAuthorizeUrl(): string {
    return "/api/v1/auth/oauth/google";
  },

  async verifyEmail(req: VerifyEmailRequest, opts?: RequestOptions): Promise<StatusResponse> {
    return await apiFetch<StatusResponse>("/api/v1/auth/verify-email", {
      method: "POST",
      body: JSON.stringify(req),
      signal: opts?.signal,
    });
  },

  async requestPasswordReset(
    req: PasswordResetRequestRequest,
    opts?: RequestOptions,
  ): Promise<StatusResponse> {
    return await apiFetch<StatusResponse>("/api/v1/auth/password-reset/request", {
      method: "POST",
      body: JSON.stringify(req),
      signal: opts?.signal,
    });
  },

  async confirmPasswordReset(
    req: PasswordResetConfirmRequest,
    opts?: RequestOptions,
  ): Promise<StatusResponse> {
    return await apiFetch<StatusResponse>("/api/v1/auth/password-reset/confirm", {
      method: "POST",
      body: JSON.stringify(req),
      signal: opts?.signal,
    });
  },

  async confirmOAuthLink(
    req: OAuthLinkConfirmRequest,
    opts?: RequestOptions,
  ): Promise<StatusResponse> {
    return await apiFetch<StatusResponse>("/api/v1/auth/oauth/link/confirm", {
      method: "POST",
      body: JSON.stringify(req),
      signal: opts?.signal,
    });
  },
};

// --------------------------------------------------------------------------
// Mock helpers & implementation
// --------------------------------------------------------------------------

function isOffline(): boolean {
  if (typeof sessionStorage !== "undefined" && sessionStorage.getItem("gs.mock.offline") === "1") {
    return true;
  }
  try {
    return new URLSearchParams(window.location.search).get("mock") === "offline";
  } catch {
    return false;
  }
}

function delay(signal?: AbortSignal): Promise<void> {
  const ms = 100 + Math.floor(Math.random() * 200);
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

function throwHttp(status: number, code: string, message?: string, details?: any): never {
  throw new ApiRequestError({
    kind: "http",
    status,
    error: { code, message: message ?? code, details },
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

const RATE_LIMIT_THRESHOLD = 5;
const RATE_LIMIT_WINDOW_MS = 30_000;

interface FailureRecord {
  count: number;
  lockedUntil: number | null;
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

export const mockImpl: AuthApi = {
  async register(req: RegisterRequest, opts?: RequestOptions): Promise<RegisterResponse> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    if (!EMAIL_PATTERN.test(req.email) || req.password.length < PASSWORD_MIN_LENGTH) {
      throwHttp(400, "VALIDATION_FAILED");
    }

    upsertUser(req.email, req.password);
    return { status: "pending_verification" };
  },

  async login(req: LoginRequest, opts?: RequestOptions): Promise<LoginResponse> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    const email = req.email.toLowerCase();
    checkRateLimit(email);

    const user = findUserByEmail(email);
    if (!user || user.password !== req.password) {
      recordFailure(email);
      const rec = getRecord(email);
      if (rec.lockedUntil !== null) {
        throwRateLimited(RATE_LIMIT_WINDOW_MS / 1000);
      }
      throwHttp(401, "AUTH_INVALID_CREDENTIALS");
    }

    resetFailures(email);
    const session = createSession(user.id);
    setCsrfToken(session.csrfToken);
    return { csrf_token: session.csrfToken };
  },

  async logout(csrfToken: string, opts?: RequestOptions): Promise<void> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    const session = getSession();
    if (!session || session.csrfToken !== csrfToken) {
      throwHttp(401, "AUTH_INVALID_TOKEN");
    }

    clearSession();
    setCsrfToken(null);
  },

  async me(opts?: RequestOptions): Promise<SessionInfo | null> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    const session = getSession();
    if (!session) return null;

    const user = sessionToCurrentUser(session);
    if (!user) return null;
    setCsrfToken(session.csrfToken);
    return { user, csrf_token: session.csrfToken };
  },

  googleAuthorizeUrl(): string {
    return "/__mock/oauth/google";
  },

  async verifyEmail(req: VerifyEmailRequest, opts?: RequestOptions): Promise<StatusResponse> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    if (!req.token || req.token === "invalid") {
      throwHttp(400, "AUTH_LINK_INVALID");
    }
    return { status: "verified" };
  },

  async requestPasswordReset(
    req: PasswordResetRequestRequest,
    opts?: RequestOptions,
  ): Promise<StatusResponse> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    if (!EMAIL_PATTERN.test(req.email)) {
      throwHttp(400, "VALIDATION_FAILED");
    }
    return { status: "sent" };
  },

  async confirmPasswordReset(
    req: PasswordResetConfirmRequest,
    opts?: RequestOptions,
  ): Promise<StatusResponse> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    if (!req.token || req.token === "invalid") {
      throwHttp(400, "AUTH_LINK_INVALID");
    }
    if (req.password.length < PASSWORD_MIN_LENGTH) {
      throwHttp(400, "VALIDATION_FAILED", undefined, [
        { field: "password", rule: "min_length" },
      ]);
    }
    if (req.password === "breachedPassword123!") {
      throwHttp(400, "VALIDATION_FAILED", undefined, [
        { field: "password", rule: "breached" },
      ]);
    }
    return { status: "reset" };
  },

  async confirmOAuthLink(
    req: OAuthLinkConfirmRequest,
    opts?: RequestOptions,
  ): Promise<StatusResponse> {
    await delay(opts?.signal);
    if (isOffline()) throwNetwork();

    if (!req.token || req.token === "invalid") {
      throwHttp(400, "AUTH_LINK_INVALID");
    }
    return { status: "linked" };
  },
};

// --------------------------------------------------------------------------
// Public export: default to realImpl unless VITE_AUTH_MOCK === "true"
// --------------------------------------------------------------------------

const useMock = import.meta.env?.VITE_AUTH_MOCK === "true";

export const authApi: AuthApi = useMock ? mockImpl : realImpl;
