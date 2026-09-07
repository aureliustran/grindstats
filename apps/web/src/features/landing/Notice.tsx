import { useTranslation } from "react-i18next";
import { X } from "lucide-react";

interface NoticeProps {
  onDismiss: () => void;
}

export function Notice({ onDismiss }: NoticeProps) {
  const { t } = useTranslation();

  return (
    <div
      role="alert"
      className="u-glass fixed inset-x-0 top-20 z-toast mx-auto flex max-w-content items-center justify-between gap-3 rounded-2xl px-5 py-3 backdrop-blur-xl backdrop-saturate-150"
    >
      <p className="text-sm text-ink-1">{t("landing.auth.error_google_cancelled")}</p>
      <button
        type="button"
        onClick={onDismiss}
        className="grid h-8 w-8 place-items-center rounded-lg text-ink-2 transition-colors duration-fast hover:bg-paper-2 hover:text-ink-0"
        aria-label={t("common.close")}
      >
        <X aria-hidden="true" className="h-4 w-4" />
      </button>
    </div>
  );
}
