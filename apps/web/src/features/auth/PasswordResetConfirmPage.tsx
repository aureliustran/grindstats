/* =============================================================================
   PasswordResetConfirmPage — sets a new password with token from emailed link.
   ============================================================================= */

import { useState, type FormEvent } from "react";
import { useSearchParams, Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { CheckCircle, AlertTriangle } from "lucide-react";
import { authApi } from "../../api/auth";
import { PASSWORD_MIN_LENGTH, ApiRequestError } from "../../api/auth.types";

export function PasswordResetConfirmPage() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token")?.trim();

  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);
  const [errorKey, setErrorKey] = useState<string | null>(null);

  if (!token) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-paper-1 p-4">
        <div className="w-full max-w-md rounded-md border border-paper-3 bg-paper-0 p-6 shadow-sm">
          <h1 className="text-xl font-semibold text-ink-0">
            {t("auth.password_reset.confirm_title")}
          </h1>
          <div className="mt-4 space-y-4">
            <div className="flex items-center gap-2 text-danger">
              <AlertTriangle className="h-5 w-5 shrink-0" aria-hidden="true" />
              <p className="text-sm">
                {t("auth.password_reset.invalid_token")}
              </p>
            </div>
            <Link
              to="/"
              className="inline-flex min-h-target items-center justify-center rounded-sm border border-paper-3 bg-paper-0 px-4 py-2 text-sm font-medium text-ink-0 transition-colors hover:bg-paper-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              {t("auth.common.back_to_home")}
            </Link>
          </div>
        </div>
      </main>
    );
  }

  const isValidPassword = password.length >= PASSWORD_MIN_LENGTH;

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!isValidPassword || loading || !token) return;

    setLoading(true);
    setErrorKey(null);

    try {
      await authApi.confirmPasswordReset({ token, password });
      setSuccess(true);
    } catch (err) {
      if (err instanceof ApiRequestError && err.failure.kind === "http") {
        const { code, details } = err.failure.error;
        if (code === "AUTH_LINK_INVALID") {
          setErrorKey("auth.password_reset.invalid_token");
        } else if (code === "VALIDATION_FAILED") {
          const rule = details?.[0]?.rule;
          if (rule === "breached") {
            setErrorKey("auth.password_reset.error_breached");
          } else if (rule === "min_length") {
            setErrorKey("auth.password_reset.error_min_length");
          } else {
            setErrorKey("auth.password_reset.generic_error");
          }
        } else {
          setErrorKey("auth.password_reset.generic_error");
        }
      } else {
        setErrorKey("common.error_internal");
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-paper-1 p-4">
      <div className="w-full max-w-md rounded-md border border-paper-3 bg-paper-0 p-6 shadow-sm">
        <h1 className="text-xl font-semibold text-ink-0">
          {t("auth.password_reset.confirm_title")}
        </h1>

        {success ? (
          <div className="mt-4 space-y-4">
            <div className="flex items-center gap-2 text-ok">
              <CheckCircle className="h-5 w-5 shrink-0" aria-hidden="true" />
              <p className="text-sm font-medium">
                {t("auth.password_reset.success_message")}
              </p>
            </div>
            <Link
              to="/"
              className="inline-flex min-h-target items-center justify-center rounded-sm bg-accent px-4 py-2 text-sm font-medium text-accent-ink transition-colors hover:bg-accent-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              {t("auth.common.back_to_login")}
            </Link>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="mt-4 space-y-4">
            {errorKey && (
              <div
                role="alert"
                className="flex items-center gap-2 rounded-sm border border-danger/20 bg-danger/10 p-3 text-sm text-danger"
              >
                <AlertTriangle className="h-4 w-4 shrink-0" aria-hidden="true" />
                <span>{t(errorKey)}</span>
              </div>
            )}

            <div>
              <label
                htmlFor="new-password"
                className="block text-sm font-medium text-ink-1"
              >
                {t("auth.password_reset.password_label")}
              </label>
              <div className="relative mt-1">
                <input
                  id="new-password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={t("auth.password_reset.password_placeholder")}
                  disabled={loading}
                  required
                  className="w-full min-h-target rounded-sm border border-paper-3 bg-paper-0 px-3 py-2 text-ink-0 placeholder:text-ink-3 focus-visible:border-accent focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent disabled:bg-paper-2"
                />
              </div>
            </div>

            <button
              type="submit"
              disabled={!isValidPassword || loading}
              className="flex w-full min-h-target items-center justify-center rounded-sm bg-accent px-4 py-2 text-sm font-medium text-accent-ink transition-colors hover:bg-accent-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:cursor-not-allowed disabled:opacity-50"
            >
              {loading ? `${t("common.loading")}...` : t("auth.password_reset.submit_confirm")}
            </button>
          </form>
        )}
      </div>
    </main>
  );
}
