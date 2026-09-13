/* =============================================================================
   Auth API contract — shapes the SPA is written against
   =============================================================================

   Source: SRS-AUTH-001 §3.7 + docs/stories/auth-epic/contract.md.
   This file IS the contract for the frontend side. It is frozen during an
   execution run; change it only through the amendment protocol in
   docs/shared-contract.md §4.

   `apps/web/src/api/auth.ts` provides the implementation (`authApi`).
   Real HTTP client is default; `VITE_AUTH_MOCK=true` uses the in-memory mock.
   Nothing outside that file may know which one it is.
   ============================================================================= */

import type { ErrorCode, Role } from "./generated/audit";

export type { ErrorCode, Role };

export interface RequestOptions {
  signal?: AbortSignal;
}

export interface ValidationDetail {
  field: string;
  rule: string;
}

export interface ApiError {
  code: ErrorCode | string;
  message?: string;
  details?: ValidationDetail[];
}

// ---- POST /api/v1/auth/register ------------------------------------------
export interface RegisterRequest {
  email: string;
  password: string;
}
/** Always 202 with this body, whether or not the email exists (FR-08). No session. */
export interface RegisterResponse {
  status: "pending_verification";
}

// ---- POST /api/v1/auth/login ---------------------------------------------
export interface LoginRequest {
  email: string;
  password: string;
}
/** 200. Tokens ride in httpOnly cookies; only the CSRF token reaches JS (FR-12). */
export interface LoginResponse {
  csrf_token: string;
}

// ---- GET /api/v1/users/me ------------------------------------------------
export interface CurrentUser {
  id: string;
  email: string;
  role: Role;
  tier: string;
  email_verified?: boolean;
}
/** 200 body. Re-issues the session-bound CSRF token (FR-12) so a reloaded SPA —
 *  which holds the token only in memory — can make state-changing calls again.
 *  Contract amendment 1, see story contract.md §0. */
export interface SessionInfo {
  user: CurrentUser;
  csrf_token: string;
}

// ---- New endpoints (contract §8.1) ----------------------------------------
export interface VerifyEmailRequest {
  token: string;
}
export interface StatusResponse {
  status: string;
}
export type VerifyEmailResponse = StatusResponse;

export interface PasswordResetRequestRequest {
  email: string;
}
export type PasswordResetRequestResponse = StatusResponse;

export interface PasswordResetConfirmRequest {
  token: string;
  password: string;
}
export type PasswordResetConfirmResponse = StatusResponse;

export interface OAuthLinkConfirmRequest {
  token: string;
}
export type OAuthLinkConfirmResponse = StatusResponse;

// ---- Failures -------------------------------------------------------------
export type ApiFailure =
  | {
      kind: "http";
      status: number;
      error: ApiError;
      /** From `Retry-After` on 429 (AUTH_RATE_LIMITED). */
      retryAfterSeconds?: number;
    }
  | { kind: "network" };

export class ApiRequestError extends Error {
  readonly failure: ApiFailure;
  constructor(failure: ApiFailure) {
    super(
      failure.kind === "http"
        ? `HTTP ${failure.status} ${failure.error.code}`
        : "network failure",
    );
    this.name = "ApiRequestError";
    this.failure = failure;
  }
}

export const isApiRequestError = (e: unknown): e is ApiRequestError =>
  e instanceof ApiRequestError;

// ---- Client interface -----------------------------------------------------
export interface AuthApi {
  register(req: RegisterRequest, opts?: RequestOptions): Promise<RegisterResponse>;
  login(req: LoginRequest, opts?: RequestOptions): Promise<LoginResponse>;
  logout(csrfToken: string, opts?: RequestOptions): Promise<void>;
  /** Session probe. Resolves `null` on 401 rather than throwing. */
  me(opts?: RequestOptions): Promise<SessionInfo | null>;
  /** URL to navigate the browser to (window.location.assign). Never fetched. */
  googleAuthorizeUrl(): string;
  verifyEmail(req: VerifyEmailRequest, opts?: RequestOptions): Promise<StatusResponse>;
  requestPasswordReset(req: PasswordResetRequestRequest, opts?: RequestOptions): Promise<StatusResponse>;
  confirmPasswordReset(req: PasswordResetConfirmRequest, opts?: RequestOptions): Promise<StatusResponse>;
  confirmOAuthLink(req: OAuthLinkConfirmRequest, opts?: RequestOptions): Promise<StatusResponse>;
}

/** Client-side validation constants (FR-02: length only, no composition rules). */
export const PASSWORD_MIN_LENGTH = 10;
export const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

