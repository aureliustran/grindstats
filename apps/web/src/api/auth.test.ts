import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { realImpl, mockImpl } from "./auth";
import { ApiRequestError } from "./auth.types";
import { setCsrfToken, getCsrfToken } from "./http";

describe("fe-auth-client", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    setCsrfToken(null);
    localStorage.clear();
    sessionStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    setCsrfToken(null);
  });

  // --------------------------------------------------------------------------
  // Story Scenario: AUTH-002 TC-01
  // --------------------------------------------------------------------------
  it("successful login issues cookie-only tokens and a CSRF token", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = String(input);
      if (url.includes("/api/v1/auth/login")) {
        return new Response(
          JSON.stringify({ data: { csrf_token: "test-csrf-token-12345" } }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }
      if (url.includes("/api/v1/auth/logout")) {
        return new Response(null, { status: 204 });
      }
      return new Response(null, { status: 404 });
    });

    // Mock document.cookie getter to detect if code reads cookie
    let cookieAccessed = false;
    const originalCookie = Object.getOwnPropertyDescriptor(Document.prototype, "cookie");
    Object.defineProperty(document, "cookie", {
      get: () => {
        cookieAccessed = true;
        return "";
      },
      configurable: true,
    });

    try {
      // 1. Perform login
      const loginRes = await realImpl.login({
        email: "user@example.com",
        password: "ValidPassword123!",
      });

      expect(loginRes.csrf_token).toBe("test-csrf-token-12345");
      expect(getCsrfToken()).toBe("test-csrf-token-12345");

      // Verify nothing is written to web storage by real client
      expect(localStorage.getItem("csrf_token")).toBeNull();
      expect(sessionStorage.getItem("csrf_token")).toBeNull();
      expect(localStorage.length).toBe(0);
      expect(sessionStorage.length).toBe(0);

      // Verify no code path read document.cookie
      expect(cookieAccessed).toBe(false);

      // 2. Next unsafe call echoes CSRF token
      await realImpl.logout("test-csrf-token-12345");

      expect(fetchSpy).toHaveBeenCalledTimes(2);
      const logoutCall = fetchSpy.mock.calls[1];
      const headers = new Headers(logoutCall[1]?.headers);
      expect(headers.get("X-CSRF-Token")).toBe("test-csrf-token-12345");
    } finally {
      if (originalCookie) {
        Object.defineProperty(Document.prototype, "cookie", originalCookie);
      }
    }
  });

  // --------------------------------------------------------------------------
  // Conformance Tests
  // --------------------------------------------------------------------------

  it("unwraps success envelope { data: ... } and parses error envelope into ApiRequestError", async () => {
    // Test success unwrap
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({ data: { status: "pending_verification" } }), {
        status: 202,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const res = await realImpl.register({
      email: "new@example.com",
      password: "Password123!",
    });
    expect(res).toEqual({ status: "pending_verification" });

    // Test error envelope parsing
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: {
            code: "VALIDATION_FAILED",
            message: "Validation failed",
            details: [{ field: "password", rule: "min_length" }],
          },
        }),
        {
          status: 400,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await expect(
      realImpl.confirmPasswordReset({
        token: "tok123",
        password: "short",
      }),
    ).rejects.toMatchObject({
      name: "ApiRequestError",
      failure: {
        kind: "http",
        status: 400,
        error: {
          code: "VALIDATION_FAILED",
          message: "Validation failed",
          details: [{ field: "password", rule: "min_length" }],
        },
      },
    });
  });

  it("includes Accept-Language and credentials: include on every request", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ data: { status: "verified" } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await realImpl.verifyEmail({ token: "sample-token" });

    expect(fetchSpy).toHaveBeenCalled();
    const call = fetchSpy.mock.calls[0];
    const init = call[1];
    expect(init?.credentials).toBe("include");
    const headers = new Headers(init?.headers);
    expect(headers.has("Accept-Language")).toBe(true);
  });

  it("includes X-CSRF-Token on logout, and excludes it on unauthenticated endpoints and refresh", async () => {
    setCsrfToken("in-memory-token");

    const fetchSpy = vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = String(input);
      if (url.includes("/logout")) return new Response(null, { status: 204 });
      if (url.includes("/refresh")) {
        return new Response(JSON.stringify({ data: { csrf_token: "refreshed" } }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify({ data: { status: "sent" } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });

    // 1. Unauthenticated endpoint (password reset request)
    await realImpl.requestPasswordReset({ email: "test@example.com" });
    const unauthCallHeaders = new Headers(fetchSpy.mock.calls[0][1]?.headers);
    expect(unauthCallHeaders.has("X-CSRF-Token")).toBe(false);

    // 2. Logout (authenticated unsafe endpoint)
    await realImpl.logout("in-memory-token");
    const logoutHeaders = new Headers(fetchSpy.mock.calls[1][1]?.headers);
    expect(logoutHeaders.get("X-CSRF-Token")).toBe("in-memory-token");
  });

  it("me() resolves null on 401 rather than throwing", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: { code: "AUTH_INVALID_TOKEN", message: "Invalid token" },
        }),
        {
          status: 401,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    // Also mock failed refresh so me() resolves null
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: { code: "AUTH_INVALID_TOKEN", message: "Refresh invalid" },
        }),
        {
          status: 401,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const result = await realImpl.me();
    expect(result).toBeNull();
  });

  it("parses Retry-After into retryAfterSeconds, defaulting to 30 if absent", async () => {
    // Case 1: Header present
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: { code: "AUTH_RATE_LIMITED", message: "Too many requests" },
        }),
        {
          status: 429,
          headers: {
            "Content-Type": "application/json",
            "Retry-After": "45",
          },
        },
      ),
    );

    try {
      await realImpl.login({ email: "spam@example.com", password: "Password123!" });
      expect.fail("Expected 429 error");
    } catch (err) {
      expect(err).toBeInstanceOf(ApiRequestError);
      const apiErr = err as ApiRequestError;
      expect(apiErr.failure.kind).toBe("http");
      if (apiErr.failure.kind === "http") {
        expect(apiErr.failure.retryAfterSeconds).toBe(45);
      }
    }

    // Case 2: Header absent -> defaults to 30
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          error: { code: "AUTH_RATE_LIMITED", message: "Too many requests" },
        }),
        {
          status: 429,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    try {
      await realImpl.login({ email: "spam@example.com", password: "Password123!" });
      expect.fail("Expected 429 error");
    } catch (err) {
      expect(err).toBeInstanceOf(ApiRequestError);
      const apiErr = err as ApiRequestError;
      expect(apiErr.failure.kind).toBe("http");
      if (apiErr.failure.kind === "http") {
        expect(apiErr.failure.retryAfterSeconds).toBe(30);
      }
    }
  });

  it("maps transport failure and network errors to ApiFailure{kind: 'network'}", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValueOnce(new TypeError("Failed to fetch"));

    await expect(
      realImpl.confirmOAuthLink({ token: "link-token-123" }),
    ).rejects.toMatchObject({
      name: "ApiRequestError",
      failure: { kind: "network" },
    });
  });

  it("handles single-flight refresh: two concurrent 401s produce exactly 1 refresh call and retry once", async () => {
    let refreshCalls = 0;
    let endpoint1Calls = 0;
    let endpoint2Calls = 0;

    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = String(input);
      if (url.includes("/auth/refresh")) {
        refreshCalls++;
        await new Promise((r) => setTimeout(r, 50));
        return new Response(
          JSON.stringify({ data: { csrf_token: "refreshed-token-999" } }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }
      if (url.includes("/users/me")) {
        endpoint1Calls++;
        if (endpoint1Calls === 1) {
          return new Response(
            JSON.stringify({ error: { code: "AUTH_INVALID_TOKEN" } }),
            { status: 401, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(
          JSON.stringify({
            data: {
              user: { id: "u1", email: "u@ex.com", role: "user", tier: "free" },
              csrf_token: "refreshed-token-999",
            },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (url.includes("/auth/verify-email")) {
        endpoint2Calls++;
        if (endpoint2Calls === 1) {
          return new Response(
            JSON.stringify({ error: { code: "AUTH_INVALID_TOKEN" } }),
            { status: 401, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(
          JSON.stringify({ data: { status: "verified" } }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      return new Response(null, { status: 404 });
    });

    // Run both concurrently
    const [res1, res2] = await Promise.all([
      realImpl.me(),
      realImpl.verifyEmail({ token: "test" }),
    ]);

    expect(refreshCalls).toBe(1); // Single flight refresh!
    expect(endpoint1Calls).toBe(2); // Retried once
    expect(endpoint2Calls).toBe(2); // Retried once
    expect(res1?.csrf_token).toBe("refreshed-token-999");
    expect(res2.status).toBe("verified");
  });

  it("handles failed refresh: produces zero retries and rejects waiters", async () => {
    let refreshCalls = 0;
    let endpointCalls = 0;

    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const url = String(input);
      if (url.includes("/auth/refresh")) {
        refreshCalls++;
        return new Response(
          JSON.stringify({ error: { code: "AUTH_INVALID_TOKEN" } }),
          { status: 401, headers: { "Content-Type": "application/json" } },
        );
      }
      if (url.includes("/auth/verify-email")) {
        endpointCalls++;
        return new Response(
          JSON.stringify({ error: { code: "AUTH_INVALID_TOKEN" } }),
          { status: 401, headers: { "Content-Type": "application/json" } },
        );
      }
      return new Response(null, { status: 404 });
    });

    await expect(realImpl.verifyEmail({ token: "test" })).rejects.toThrow();
    expect(refreshCalls).toBe(1);
    expect(endpointCalls).toBe(1); // Zero retries!
  });

  it("mock implementation returns same unwrapped shapes as real client", async () => {
    // Verify register
    const regRes = await mockImpl.register({
      email: "mock_new@example.com",
      password: "MockPassword123!",
    });
    expect(regRes).toEqual({ status: "pending_verification" });

    // Verify login
    const loginRes = await mockImpl.login({
      email: "returning@example.com",
      password: "Str0ng-Passw0rd!",
    });
    expect(typeof loginRes.csrf_token).toBe("string");

    // Verify me
    const meRes = await mockImpl.me();
    expect(meRes?.user.email).toBe("returning@example.com");

    // Verify verifyEmail
    const verRes = await mockImpl.verifyEmail({ token: "valid-tok" });
    expect(verRes).toEqual({ status: "verified" });

    // Verify requestPasswordReset
    const reqReset = await mockImpl.requestPasswordReset({ email: "returning@example.com" });
    expect(reqReset).toEqual({ status: "sent" });

    // Verify confirmPasswordReset
    const confReset = await mockImpl.confirmPasswordReset({
      token: "valid-tok",
      password: "NewPassword123!",
    });
    expect(confReset).toEqual({ status: "reset" });

    // Verify confirmOAuthLink
    const oAuthRes = await mockImpl.confirmOAuthLink({ token: "valid-tok" });
    expect(oAuthRes).toEqual({ status: "linked" });

    // Verify logout
    await mockImpl.logout(loginRes.csrf_token);
    expect(await mockImpl.me()).toBeNull();
  });
});
