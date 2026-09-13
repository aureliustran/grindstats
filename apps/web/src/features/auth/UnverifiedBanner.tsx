/* =============================================================================
   UnverifiedBanner — persistent notice for unverified authenticated accounts.
   Adheres to tokens.css and design-system rules (no raw hex/px/ms).
   ============================================================================= */

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { AlertCircle, X } from "lucide-react";

export interface UnverifiedBannerProps {
  emailVerified?: boolean;
  onDismiss?: () => void;
  className?: string;
}

const STORAGE_KEY = "gs.unverified_banner.dismissed";

export function UnverifiedBanner({
  emailVerified = true,
  onDismiss,
  className = "",
}: UnverifiedBannerProps) {
  const { t } = useTranslation();
  const [dismissed, setDismissed] = useState(() => {
    if (typeof sessionStorage === "undefined") return false;
    return sessionStorage.getItem(STORAGE_KEY) === "1";
  });

  // If the account is verified or dismissed for this session, render nothing
  if (emailVerified || dismissed) {
    return null;
  }

  function handleDismiss() {
    setDismissed(true);
    if (typeof sessionStorage !== "undefined") {
      sessionStorage.setItem(STORAGE_KEY, "1");
    }
    onDismiss?.();
  }

  return (
    <aside
      role="alert"
      aria-live="polite"
      className={`flex items-center justify-between gap-3 border-b border-warn/30 bg-warn/10 px-4 py-3 text-sm text-ink-0 ${className}`}
    >
      <div className="flex items-center gap-2">
        <AlertCircle className="h-4 w-4 shrink-0 text-warn" aria-hidden="true" />
        <span>{t("auth.unverified.banner")}</span>
      </div>
      <button
        type="button"
        onClick={handleDismiss}
        aria-label={t("auth.unverified.dismiss")}
        className="flex min-h-target min-w-target items-center justify-center rounded-sm text-ink-2 transition-colors hover:text-ink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
      >
        <X className="h-4 w-4" aria-hidden="true" />
      </button>
    </aside>
  );
}
