/* =============================================================================
   HTTP client wrapper — transport rules, envelope unwrapping, single-flight refresh
   =============================================================================

   Contract reference: docs/stories/auth-epic/contract.md §8.2, §1, §2.3.
   ============================================================================= */

import i18n from "../i18n";
import { ApiRequestError, type ApiFailure } from "./auth.types";

/** In-memory CSRF token storage (never written to localStorage or sessionStorage). */
let inMemoryCsrfToken: string | null = null;

export function getCsrfToken(): string | null {
  return inMemoryCsrfToken;
}

export function setCsrfToken(token: string | null): void {
  inMemoryCsrfToken = token;
}

/** Endpoints that are unauthenticated and do not require X-CSRF-Token. */
const CSRF_EXEMPT_PATHS = new Set([
  "/api/v1/auth/register",
  "/api/v1/auth/login",
  "/api/v1/auth/verify-email",
  "/api/v1/auth/password-reset/request",
  "/api/v1/auth/password-reset/confirm",
  "/api/v1/auth/oauth/link/confirm",
  "/api/v1/auth/refresh", // Contract D7
]);

/** Shared in-flight promise for single-flight reactive refresh. */
let inFlightRefresh: Promise<string> | null = null;

interface RequestConfig extends RequestInit {
  _isRetry?: boolean;
}

/**
 * Executes an HTTP request adhering to the contract:
 * - credentials: "include" on every request
 * - Accept-Language: <resolvedLanguage> on every request
 * - X-CSRF-Token on unsafe requests (except exempt paths)
 * - Enveloped response parsing: unwraps { data: T }, parses { error: ... }
 * - Single-flight reactive refresh on 401 AUTH_INVALID_TOKEN
 */
export async function apiFetch<T>(
  path: string,
  init?: RequestConfig,
): Promise<T> {
  const method = (init?.method ?? "GET").toUpperCase();
  const headers = new Headers(init?.headers);

  // 1. Transport rule: Accept-Language
  const lang = i18n.resolvedLanguage || "en-US";
  if (!headers.has("Accept-Language")) {
    headers.set("Accept-Language", lang);
  }

  // 2. Transport rule: Content-Type default for JSON bodies
  if (init?.body && typeof init.body === "string" && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  // 3. Transport rule: X-CSRF-Token on unsafe requests (unless exempt)
  const isUnsafe = ["POST", "PUT", "PATCH", "DELETE"].includes(method);
  if (isUnsafe && !CSRF_EXEMPT_PATHS.has(path)) {
    if (inMemoryCsrfToken) {
      headers.set("X-CSRF-Token", inMemoryCsrfToken);
    }
  }

  const fetchInit: RequestInit = {
    ...init,
    method,
    headers,
    credentials: "include", // Contract: credentials "include" on every request
  };

  let response: Response;
  try {
    response = await fetch(path, fetchInit);
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      throw err;
    }
    // Network failure or abort
    throw new ApiRequestError({ kind: "network" });
  }

  // 4. Handle 401 and single-flight reactive refresh
  if (response.status === 401 && !init?._isRetry && path !== "/api/v1/auth/refresh") {
    let errorData: any = null;
    try {
      errorData = await response.clone().json();
    } catch {
      // ignore clone read error
    }

    const errorCode = errorData?.error?.code;

    // Terminal 401 AUTH_SESSION_EXPIRED: do not refresh, clear session
    if (errorCode === "AUTH_SESSION_EXPIRED") {
      setCsrfToken(null);
      throw parseHttpError(response, errorData);
    }

    // 401 AUTH_INVALID_TOKEN triggers single-flight refresh
    if (errorCode === "AUTH_INVALID_TOKEN" || !errorCode) {
      try {
        await executeSingleFlightRefresh();
        // Retry original request exactly once
        return await apiFetch<T>(path, { ...init, _isRetry: true });
      } catch {
        // Refresh failed: resolve waiters as unauthenticated, do not retry
        setCsrfToken(null);
        throw parseHttpError(response, errorData);
      }
    }
  }

  // 5. Handle HTTP Errors
  if (!response.ok) {
    let errorData: any = null;
    try {
      errorData = await response.json();
    } catch {
      // Body not JSON
    }
    throw parseHttpError(response, errorData);
  }

  // 6. Handle 204 No Content
  if (response.status === 204) {
    return undefined as unknown as T;
  }

  // 7. Envelope unwrapping: { data: T }
  const json = await response.json();
  if (json && typeof json === "object" && "data" in json) {
    return json.data as T;
  }

  return json as T;
}

/** Single-flight refresh coordination */
async function executeSingleFlightRefresh(): Promise<string> {
  if (inFlightRefresh) {
    return inFlightRefresh;
  }

  inFlightRefresh = (async () => {
    try {
      const res = await fetch("/api/v1/auth/refresh", {
        method: "POST",
        credentials: "include",
        headers: {
          "Accept-Language": i18n.resolvedLanguage || "en-US",
        },
      });

      if (!res.ok) {
        throw new Error(`Refresh failed: ${res.status}`);
      }

      const json = await res.json();
      const token = json?.data?.csrf_token;
      if (token && typeof token === "string") {
        setCsrfToken(token);
        return token;
      }
      throw new Error("No CSRF token returned from refresh");
    } finally {
      inFlightRefresh = null;
    }
  })();

  return inFlightRefresh;
}

function parseHttpError(res: Response, jsonBody: any): ApiRequestError {
  let retryAfterSeconds: number | undefined;
  if (res.status === 429) {
    const headerVal = res.headers.get("Retry-After");
    if (headerVal) {
      const parsed = parseInt(headerVal, 10);
      retryAfterSeconds = isNaN(parsed) ? 30 : parsed;
    } else {
      retryAfterSeconds = 30; // Default 30s per contract §8.2
    }
  }

  const errObj = jsonBody?.error;
  const failure: ApiFailure = {
    kind: "http",
    status: res.status,
    error: {
      code: errObj?.code ?? `HTTP_${res.status}`,
      message: errObj?.message ?? res.statusText,
      details: errObj?.details,
    },
    ...(retryAfterSeconds !== undefined ? { retryAfterSeconds } : {}),
  };

  return new ApiRequestError(failure);
}
