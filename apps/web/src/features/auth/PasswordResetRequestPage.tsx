/* =============================================================================
   PasswordResetRequestPage — handles requesting password reset link.
   Security requirement (FR-08): identical confirmation whether or not email exists.
   ============================================================================= */

import { useState, useEffect, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { CheckCircle, AlertTriangle } from "lucide-react";
import { authApi } from "../../api/auth";
import { EMAIL_PATTERN, ApiRequestError } from "../../api/auth.types";

export function PasswordResetRequestPage() {
  const { t, i18n } = useTranslation();
  const [email, setEmail] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [loading, setLoading] = useState(false);
  const [rateLimitEnd, setRateLimitEnd] = useState<number | null>(null);
  const [rateLimitCountdown, setRateLimitCountdown] = useState(0);

  // Rate limiting countdown
  useEffect(() => {
    if (rateLimitEnd === null) return;
    const tick = () => {
      const remaining = Math.max(0, Math.ceil((rateLimitEnd - Date.now()) / 1000));
      setRateLimitCountdown(remaining);
      if (remaining === 0) setRateLimitEnd(null);
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [rateLimitEnd]);

  const isRateLimited = rateLimitEnd !== null && rateLimitCountdown > 0;
  const isValidEmail = EMAIL_PATTERN.test(email.trim());

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!isValidEmail || loading || isRateLimited) return;

    setLoading(true);
    try {
      await authApi.requestPasswordReset({ email: email.trim() });
      setSubmitted(true);
    } catch (err) {
      if (err instanceof ApiRequestError && err.failure.kind === "http" && err.failure.status === 429) {
        const seconds = err.failure.retryAfterSeconds ?? 30;
        setRateLimitEnd(Date.now() + seconds * 1000);
      } else {
        // Even on failure (except rate-limited), show the same neutral confirmation per FR-08
        setSubmitted(true);
      }
    } finally {
      setLoading(false);
    }
  }

  // Format countdown number through Intl.NumberFormat per design system
  const formattedCountdown = new Intl.NumberFormat(i18n.resolvedLanguage || "en-US").format(
    rateLimitCountdown,
  );

  return (
    <main className="flex min-h-screen items-center justify-center bg-paper-1 p-4">
      <div className="w-full max-w-md rounded-md border border-paper-3 bg-paper-0 p-6 shadow-sm">
        <h1 className="text-xl font-semibold text-ink-0">
          {t("auth.password_reset.request_title")}
        </h1>

        {submitted ? (
          <div className="mt-4 space-y-4">
            <div className="flex items-start gap-2 text-ink-1">
              <CheckCircle className="h-5 w-5 shrink-0 text-ok" aria-hidden="true" />
              <p className="text-sm">
                {t("auth.password_reset.request_sent_notice")}
              </p>
            </div>
            <Link
              to="/"
              className="inline-flex min-h-target items-center justify-center rounded-sm bg-accent px-4 py-2 text-sm font-medium text-accent-ink transition-colors hover:bg-accent-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              {t("auth.common.back_to_home")}
            </Link>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="mt-4 space-y-4">
            {isRateLimited && (
              <div
                role="alert"
                className="flex items-center gap-2 rounded-sm border border-danger/20 bg-danger/10 p-3 text-sm text-danger"
              >
                <AlertTriangle className="h-4 w-4 shrink-0" aria-hidden="true" />
                <span className="font-mono">
                  {t("auth.password_reset.rate_limited", { seconds: formattedCountdown })}
                </span>
              </div>
            )}

            <div>
              <label
                htmlFor="reset-email"
                className="block text-sm font-medium text-ink-1"
              >
                {t("auth.password_reset.email_label")}
              </label>
              <div className="relative mt-1">
                <input
                  id="reset-email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder={t("auth.password_reset.email_placeholder")}
                  disabled={loading || isRateLimited}
                  required
                  className="w-full min-h-target rounded-sm border border-paper-3 bg-paper-0 px-3 py-2 text-ink-0 placeholder:text-ink-3 focus-visible:border-accent focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-accent disabled:bg-paper-2"
                />
              </div>
            </div>

            <button
              type="submit"
              disabled={!isValidEmail || loading || isRateLimited}
              className="flex w-full min-h-target items-center justify-center rounded-sm bg-accent px-4 py-2 text-sm font-medium text-accent-ink transition-colors hover:bg-accent-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:cursor-not-allowed disabled:opacity-50"
            >
              {loading ? `${t("common.loading")}...` : t("auth.password_reset.submit_request")}
            </button>

            <div className="text-center">
              <Link
                to="/"
                className="text-xs text-ink-2 hover:text-ink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
              >
                {t("auth.common.back_to_login")}
              </Link>
            </div>
          </form>
        )}
      </div>
    </main>
  );
}
