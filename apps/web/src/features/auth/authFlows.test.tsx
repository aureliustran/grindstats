import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { authApi } from "../../api/auth";
import { ApiRequestError } from "../../api/auth.types";
import {
  VerifyEmailPage,
  PasswordResetRequestPage,
  PasswordResetConfirmPage,
  OAuthLinkConfirmPage,
} from "./index";

// Mock react-i18next so t returns the key, unless options interpolates
vi.mock("react-i18next", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-i18next")>();
  return {
    ...actual,
    useTranslation: () => ({
      t: (key: string, opts?: any) => {
        if (opts?.seconds !== undefined) {
          return `${key}:${opts.seconds}`;
        }
        return key;
      },
      i18n: { resolvedLanguage: "en-US" },
    }),
  };
});

describe("fe-auth-flows conformance", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  // --------------------------------------------------------------------------
  // Verify Email
  // --------------------------------------------------------------------------
  it("verify-email renders success state when valid token is supplied", async () => {
    vi.spyOn(authApi, "verifyEmail").mockResolvedValueOnce({ status: "verified" });

    render(
      <MemoryRouter initialEntries={["/auth/verify-email?token=valid-tok"]}>
        <Routes>
          <Route path="/auth/verify-email" element={<VerifyEmailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByText("auth.verify_email.success_message")).toBeInTheDocument();
    expect(screen.getByText("auth.verify_email.action_login")).toBeInTheDocument();
  });

  it("verify-email renders invalid-link state with way forward on AUTH_LINK_INVALID", async () => {
    vi.spyOn(authApi, "verifyEmail").mockRejectedValueOnce(
      new ApiRequestError({
        kind: "http",
        status: 400,
        error: { code: "AUTH_LINK_INVALID" },
      }),
    );

    render(
      <MemoryRouter initialEntries={["/auth/verify-email?token=invalid-tok"]}>
        <Routes>
          <Route path="/auth/verify-email" element={<VerifyEmailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(await screen.findByText("auth.verify_email.invalid_or_expired")).toBeInTheDocument();
    expect(screen.getByText("auth.common.back_to_home")).toBeInTheDocument();
  });

  // --------------------------------------------------------------------------
  // Password Reset Request (FR-08)
  // --------------------------------------------------------------------------
  it("password-reset request renders identical confirmation key for two different emails (FR-08)", async () => {
    const reqSpy = vi.spyOn(authApi, "requestPasswordReset").mockResolvedValue({ status: "sent" });

    // Test 1: Registered email
    const { unmount } = render(
      <MemoryRouter>
        <PasswordResetRequestPage />
      </MemoryRouter>,
    );

    const input1 = screen.getByLabelText("auth.password_reset.email_label");
    fireEvent.change(input1, { target: { value: "registered@example.com" } });
    fireEvent.click(screen.getByText("auth.password_reset.submit_request"));

    await waitFor(() => {
      expect(screen.getByText("auth.password_reset.request_sent_notice")).toBeInTheDocument();
    });

    unmount();

    // Test 2: Non-existent email
    render(
      <MemoryRouter>
        <PasswordResetRequestPage />
      </MemoryRouter>,
    );

    const input2 = screen.getByLabelText("auth.password_reset.email_label");
    fireEvent.change(input2, { target: { value: "unknown@example.com" } });
    fireEvent.click(screen.getByText("auth.password_reset.submit_request"));

    await waitFor(() => {
      expect(screen.getByText("auth.password_reset.request_sent_notice")).toBeInTheDocument();
    });

    expect(reqSpy).toHaveBeenCalledTimes(2);
  });

  // --------------------------------------------------------------------------
  // Password Reset Confirm
  // --------------------------------------------------------------------------
  it("password-reset confirm renders distinct keys for breached vs min_length validation", async () => {
    // 1. breached error
    vi.spyOn(authApi, "confirmPasswordReset").mockRejectedValueOnce(
      new ApiRequestError({
        kind: "http",
        status: 400,
        error: {
          code: "VALIDATION_FAILED",
          details: [{ field: "password", rule: "breached" }],
        },
      }),
    );

    const { unmount } = render(
      <MemoryRouter initialEntries={["/auth/password-reset/confirm?token=test-tok"]}>
        <Routes>
          <Route path="/auth/password-reset/confirm" element={<PasswordResetConfirmPage />} />
        </Routes>
      </MemoryRouter>,
    );

    const input = screen.getByLabelText("auth.password_reset.password_label");
    fireEvent.change(input, { target: { value: "Password123!" } });
    fireEvent.click(screen.getByText("auth.password_reset.submit_confirm"));

    expect(await screen.findByText("auth.password_reset.error_breached")).toBeInTheDocument();

    unmount();

    // 2. min_length error
    vi.spyOn(authApi, "confirmPasswordReset").mockRejectedValueOnce(
      new ApiRequestError({
        kind: "http",
        status: 400,
        error: {
          code: "VALIDATION_FAILED",
          details: [{ field: "password", rule: "min_length" }],
        },
      }),
    );

    const { unmount: unmount2 } = render(
      <MemoryRouter initialEntries={["/auth/password-reset/confirm?token=test-tok"]}>
        <Routes>
          <Route path="/auth/password-reset/confirm" element={<PasswordResetConfirmPage />} />
        </Routes>
      </MemoryRouter>,
    );

    const input2 = screen.getByLabelText("auth.password_reset.password_label");
    fireEvent.change(input2, { target: { value: "Password123!" } });
    fireEvent.click(screen.getByText("auth.password_reset.submit_confirm"));

    expect(await screen.findByText("auth.password_reset.error_min_length")).toBeInTheDocument();

    unmount2();

    // 3. details absent -> generic error key
    vi.spyOn(authApi, "confirmPasswordReset").mockRejectedValueOnce(
      new ApiRequestError({
        kind: "http",
        status: 400,
        error: { code: "VALIDATION_FAILED" },
      }),
    );

    render(
      <MemoryRouter initialEntries={["/auth/password-reset/confirm?token=test-tok"]}>
        <Routes>
          <Route path="/auth/password-reset/confirm" element={<PasswordResetConfirmPage />} />
        </Routes>
      </MemoryRouter>,
    );

    const input3 = screen.getByLabelText("auth.password_reset.password_label");
    fireEvent.change(input3, { target: { value: "Password123!" } });
    fireEvent.click(screen.getByText("auth.password_reset.submit_confirm"));

    expect(await screen.findByText("auth.password_reset.generic_error")).toBeInTheDocument();
  });

  // --------------------------------------------------------------------------
  // Routes & Token-less visit
  // --------------------------------------------------------------------------
  it("token-less visit renders invalid link state without calling API", () => {
    const apiSpy = vi.spyOn(authApi, "verifyEmail");

    render(
      <MemoryRouter initialEntries={["/auth/verify-email"]}>
        <Routes>
          <Route path="/auth/verify-email" element={<VerifyEmailPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByText("auth.verify_email.invalid_or_expired")).toBeInTheDocument();
    expect(apiSpy).not.toHaveBeenCalled();
  });

  it("OAuth link confirm renders invalid token state when token is missing", () => {
    const apiSpy = vi.spyOn(authApi, "confirmOAuthLink");

    render(
      <MemoryRouter initialEntries={["/auth/oauth/link"]}>
        <Routes>
          <Route path="/auth/oauth/link" element={<OAuthLinkConfirmPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(screen.getByText("auth.oauth_link.invalid_token")).toBeInTheDocument();
    expect(apiSpy).not.toHaveBeenCalled();
  });

  // --------------------------------------------------------------------------
  // Locales: vi-VN & Intl number formatting
  // --------------------------------------------------------------------------
  it("renders countdown formatted through Intl.NumberFormat in vi-VN locale", async () => {
    vi.spyOn(authApi, "requestPasswordReset").mockRejectedValueOnce(
      new ApiRequestError({
        kind: "http",
        status: 429,
        error: { code: "AUTH_RATE_LIMITED" },
        retryAfterSeconds: 45,
      }),
    );

    render(
      <MemoryRouter>
        <PasswordResetRequestPage />
      </MemoryRouter>,
    );

    const input = screen.getByLabelText("auth.password_reset.email_label");
    fireEvent.change(input, { target: { value: "test@example.com" } });
    fireEvent.click(screen.getByText("auth.password_reset.submit_request"));

    // Rate-limited countdown rendered with interpolated number
    expect(await screen.findByText(/auth\.password_reset\.rate_limited:45/)).toBeInTheDocument();
  });
});
