import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { useTranslation } from "react-i18next";
import { ArrowRight, Lock, Mail, X } from "lucide-react";
import { authApi } from "../../api/auth";
import {
  EMAIL_PATTERN,
  PASSWORD_MIN_LENGTH,
  ApiRequestError,
} from "../../api/auth.types";
import { errorMessageKey } from "../../i18n/errorMessages";

type ModalTab = "login" | "signup";

interface AuthModalProps {
  initialTab: ModalTab;
  onClose: () => void;
  onAuthenticated: (session: { csrfToken: string }) => void;
}

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function AuthModal({
  initialTab,
  onClose,
  onAuthenticated,
}: AuthModalProps) {
  const { t } = useTranslation();
  const dialogRef = useRef<HTMLDivElement>(null);
  const emailInputRef = useRef<HTMLInputElement>(null);

  const [tab, setTab] = useState<ModalTab>(initialTab);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const [emailTouched, setEmailTouched] = useState(false);
  const [passwordTouched, setPasswordTouched] = useState(false);

  const [apiError, setApiError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [rateLimitEnd, setRateLimitEnd] = useState<number | null>(null);
  const [rateLimitCountdown, setRateLimitCountdown] = useState(0);
  const [signupSuccess, setSignupSuccess] = useState(false);

  // Rate limiting countdown
  useEffect(() => {
    if (rateLimitEnd === null) return;
    const tick = () => {
      const remaining = Math.max(
        0,
        Math.ceil((rateLimitEnd - Date.now()) / 1000),
      );
      setRateLimitCountdown(remaining);
      if (remaining === 0) setRateLimitEnd(null);
    };
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [rateLimitEnd]);

  const isRateLimited = rateLimitEnd !== null && rateLimitCountdown > 0;

  // Lock body scroll
  useEffect(() => {
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = "";
    };
  }, []);

  // Initial focus
  useEffect(() => {
    emailInputRef.current?.focus();
  }, [tab]);

  // Focus trap & Escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();
        return;
      }
      if (e.key !== "Tab") return;

      const focusable = Array.from(
        dialogRef.current?.querySelectorAll<HTMLElement>(FOCUSABLE) ?? [],
      );
      if (focusable.length === 0) return;

      const first = focusable[0];
      const last = focusable[focusable.length - 1];

      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const emailValid = EMAIL_PATTERN.test(email);
  const passwordValid = password.length >= PASSWORD_MIN_LENGTH;
  const isFormValid = emailValid && passwordValid;

  const switchTab = (nextTab: ModalTab) => {
    setTab(nextTab);
    setApiError(null);
    setEmailTouched(false);
    setPasswordTouched(false);
    setSignupSuccess(false);
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setEmailTouched(true);
    setPasswordTouched(true);

    if (!isFormValid || loading || isRateLimited) return;

    setLoading(true);
    setApiError(null);

    if (tab === "login") {
      try {
        const result = await authApi.login({ email, password });
        onAuthenticated({ csrfToken: result.csrf_token });
        onClose();
      } catch (err) {
        if (err instanceof ApiRequestError) {
          if (err.failure.kind === "http") {
            // Password is cleared on any HTTP-level failure (wrong
            // credentials, rate-limited, ...) per the AC: "the password
            // field is cleared while the email field keeps its value".
            // A network failure leaves it in place so the visitor can retry
            // without retyping.
            setPassword("");
            setPasswordTouched(false);
            if (err.failure.status === 429) {
              const retryAfterSec = err.failure.retryAfterSeconds ?? 30;
              setRateLimitEnd(Date.now() + retryAfterSec * 1000);
              setRateLimitCountdown(retryAfterSec);
            } else {
              const key = errorMessageKey(err.failure.error);
              setApiError(key ? t(key) : (err.failure.error.message ?? t("landing.auth.error_unreachable")));
            }
          } else {
            setApiError(t("landing.auth.error_unreachable"));
          }
        } else {
          setApiError(t("landing.auth.error_unreachable"));
        }
      } finally {
        setLoading(false);
      }
    } else {
      // signup
      try {
        const result = await authApi.register({ email, password });
        if (result.status === "pending_verification") {
          setSignupSuccess(true);
        }
      } catch (err) {
        if (err instanceof ApiRequestError) {
          if (err.failure.kind === "http") {
            // Password is cleared on any HTTP-level failure (wrong
            // credentials, rate-limited, ...) per the AC: "the password
            // field is cleared while the email field keeps its value".
            // A network failure leaves it in place so the visitor can retry
            // without retyping.
            setPassword("");
            setPasswordTouched(false);
            if (err.failure.status === 429) {
              const retryAfterSec = err.failure.retryAfterSeconds ?? 30;
              setRateLimitEnd(Date.now() + retryAfterSec * 1000);
              setRateLimitCountdown(retryAfterSec);
            } else {
              const key = errorMessageKey(err.failure.error);
              setApiError(key ? t(key) : (err.failure.error.message ?? t("landing.auth.error_unreachable")));
            }
          } else {
            setApiError(t("landing.auth.error_unreachable"));
          }
        } else {
          setApiError(t("landing.auth.error_unreachable"));
        }
      } finally {
        setLoading(false);
      }
    }
  };

  const handleGoogleClick = () => {
    window.location.assign(authApi.googleAuthorizeUrl());
  };

  const submitDisabled = !isFormValid || loading || isRateLimited;

  return (
    <div
      className="modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="authTitle"
    >
      <div
        className="modal__backdrop"
        onClick={onClose}
        aria-hidden="true"
      />

      <div
        ref={dialogRef}
        className="glass-card modal__panel rounded-3xl p-6 md:p-8 relative"
      >
        <button
          type="button"
          onClick={onClose}
          className="icon-btn absolute right-4 top-4"
          aria-label={t("common.close")}
        >
          <X aria-hidden="true" className="h-4 w-4" />
        </button>

        <h2
          id="authTitle"
          className="font-display text-3xl font-black uppercase text-ink-0"
        >
          {t("landing.auth.dialog_label")}
        </h2>

        {/* Segmented Tab Switcher */}
        <div
          className="segmented mt-5"
          role="tablist"
          data-active={tab}
        >
          <button
            type="button"
            role="tab"
            data-tab="login"
            aria-selected={tab === "login"}
            onClick={() => switchTab("login")}
          >
            {t("landing.auth.tab_login")}
          </button>
          <button
            type="button"
            role="tab"
            data-tab="signup"
            aria-selected={tab === "signup"}
            onClick={() => switchTab("signup")}
          >
            {t("landing.auth.tab_signup")}
          </button>
          <span className="segmented__thumb" aria-hidden="true" />
        </div>

        {signupSuccess ? (
          <div className="mt-6 space-y-4">
            <div className="rounded-xl border border-ok/40 bg-ok/10 p-4 text-sm text-ok">
              <p className="font-semibold">{t("landing.auth.signup_next_step")}</p>
              <p className="mt-1 text-xs text-ink-2">
                {t("landing.auth.signup_check_inbox", "We sent a verification link to your email. Click it to confirm your account, then log in.")}
              </p>
            </div>
            <button
              type="button"
              onClick={() => switchTab("login")}
              className="btn-primary btn-lg w-full"
            >
              {t("landing.auth.tab_login")}{" "}
              <ArrowRight aria-hidden="true" className="h-4 w-4" />
            </button>
          </div>
        ) : (
          <form className="mt-6 space-y-4" onSubmit={handleSubmit} noValidate>
            <label className="field">
              <span>{t("landing.auth.email_label")}</span>
              <div className="field__input">
                <Mail aria-hidden="true" className="h-4 w-4 flex-none" />
                <input
                  ref={emailInputRef}
                  type="email"
                  value={email}
                  placeholder={t("landing.auth.email_placeholder")}
                  autoComplete="email"
                  onChange={(e) => {
                    setEmail(e.target.value);
                    if (apiError) setApiError(null);
                  }}
                  onBlur={() => setEmailTouched(true)}
                  disabled={loading}
                  required
                />
              </div>
              {emailTouched && !emailValid && (
                <p className="text-xs text-danger" role="alert">
                  {t("landing.auth.error_email_invalid")}
                </p>
              )}
            </label>

            <label className="field">
              <span>{t("landing.auth.password_label")}</span>
              <div className="field__input">
                <Lock aria-hidden="true" className="h-4 w-4 flex-none" />
                <input
                  type="password"
                  value={password}
                  placeholder="••••••••••"
                  autoComplete={tab === "login" ? "current-password" : "new-password"}
                  onChange={(e) => {
                    setPassword(e.target.value);
                    if (apiError) setApiError(null);
                  }}
                  onBlur={() => setPasswordTouched(true)}
                  disabled={loading}
                  required
                />
              </div>
              <small className="text-ink-2 text-xs">
                {t("landing.auth.password_hint", { count: PASSWORD_MIN_LENGTH })}
              </small>
              {passwordTouched && !passwordValid && (
                <p className="text-xs text-danger" role="alert">
                  {t("landing.auth.error_password_short", { count: PASSWORD_MIN_LENGTH })}
                </p>
              )}
            </label>

            {isRateLimited && (
              <p className="text-xs text-danger" role="alert">
                {t("landing.auth.error_rate_limited")}{" "}
                <span className="font-mono">({rateLimitCountdown}s)</span>
              </p>
            )}

            {apiError && !isRateLimited && (
              <p className="text-xs text-danger" role="alert">
                {apiError}
              </p>
            )}

            <button
              type="submit"
              disabled={submitDisabled}
              className="btn-primary btn-lg w-full"
            >
              {loading
                ? t("common.loading")
                : tab === "login"
                  ? t("landing.auth.submit_login")
                  : t("landing.auth.submit_signup")}{" "}
              <ArrowRight aria-hidden="true" className="h-4 w-4" />
            </button>
          </form>
        )}

        {/* Divider */}
        <div className="my-5 flex items-center gap-3 text-xs uppercase tracking-widest text-ink-3">
          <span className="h-px flex-1 bg-paper-3/60" />
          {t("landing.auth.divider")}
          <span className="h-px flex-1 bg-paper-3/60" />
        </div>

        {/* Google OAuth button */}
        <button
          type="button"
          onClick={handleGoogleClick}
          className="btn-ghost w-full"
        >
          <svg
            className="h-4 w-4"
            viewBox="0 0 24 24"
            aria-hidden="true"
          >
            <path
              fill="currentColor"
              d="M21.6 12.2c0-.7-.1-1.3-.2-1.9H12v3.7h5.4a4.6 4.6 0 0 1-2 3v2.5h3.2c1.9-1.7 3-4.3 3-7.3Z"
            />
            <path
              fill="currentColor"
              opacity=".7"
              d="M12 22c2.7 0 5-.9 6.6-2.4l-3.2-2.5c-.9.6-2 1-3.4 1-2.6 0-4.8-1.8-5.6-4.1H3.1v2.6A10 10 0 0 0 12 22Z"
            />
            <path
              fill="currentColor"
              opacity=".5"
              d="M6.4 14a6 6 0 0 1 0-3.9V7.5H3.1a10 10 0 0 0 0 9l3.3-2.5Z"
            />
            <path
              fill="currentColor"
              opacity=".85"
              d="M12 6c1.5 0 2.8.5 3.8 1.5l2.8-2.8A10 10 0 0 0 3.1 7.5L6.4 10C7.2 7.8 9.4 6 12 6Z"
            />
          </svg>
          {t("landing.auth.google_cta")}
        </button>
      </div>
    </div>
  );
}
