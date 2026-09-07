/* =============================================================================
   Root error boundary.
   Uses existing common.* / app.not_found.* keys only.
   ============================================================================= */

import { Component, type ErrorInfo, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
}

/* Fallback is a functional component so it can use the useTranslation hook. */
function ErrorFallback() {
  const { t } = useTranslation();
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-4 bg-paper-1 p-4">
      <h1 className="text-2xl font-bold text-ink-0">{t("app.not_found.heading")}</h1>
      <a href="/" className="text-base text-accent underline">
        {t("app.not_found.back")}
      </a>
    </main>
  );
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(): State {
    return { hasError: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("[ErrorBoundary]", error, info.componentStack);
  }

  render() {
    if (this.state.hasError) {
      return <ErrorFallback />;
    }
    return this.props.children;
  }
}
