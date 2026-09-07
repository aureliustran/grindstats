/* =============================================================================
   Application router.

   Routes:
     /                     — landing (or redirect to /dashboard if authenticated)
     /dashboard            — protected placeholder
     /__mock/oauth/google  — mock consent page (mock-only, conditional)
     *                     — not found

   Document title is set per-route using the i18n keys:
     /           → landing.meta.title
     everything  → common.app_name
   ============================================================================= */

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import {
  BrowserRouter,
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
  useSearchParams,
} from "react-router-dom";
import { LandingPage } from "../features/landing";
import type { LandingNotice } from "../features/landing";
import { useAuth } from "./AuthContext";
import { DashboardPage } from "./pages/DashboardPage";
import { MockOAuthPage } from "./pages/MockOAuthPage";
import { NotFoundPage } from "./pages/NotFoundPage";

// --------------------------------------------------------------------------
// Whether the mock is active (determines if /__mock/oauth/google is registered)
// --------------------------------------------------------------------------

const MOCK_ACTIVE =
  !import.meta.env.VITE_AUTH_MOCK || import.meta.env.VITE_AUTH_MOCK === "true";

// --------------------------------------------------------------------------
// Document title manager
// --------------------------------------------------------------------------

function TitleManager() {
  const { t } = useTranslation();
  const { pathname } = useLocation();

  useEffect(() => {
    document.title =
      pathname === "/" ? t("landing.meta.title") : t("common.app_name");
  }, [pathname, t]);

  return null;
}

// --------------------------------------------------------------------------
// Loading indicator
// --------------------------------------------------------------------------

function LoadingScreen() {
  const { t } = useTranslation();
  return (
    <main className="flex min-h-screen items-center justify-center bg-paper-1 p-4">
      <p className="text-base text-ink-2">{t("common.loading")}</p>
    </main>
  );
}

// --------------------------------------------------------------------------
// Landing route — reads notice from ?auth_error, dismisses it via setSearchParams
// --------------------------------------------------------------------------

function LandingRoute() {
  const { status, markAuthenticated } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();

  if (status === "loading") return <LoadingScreen />;
  if (status === "authenticated") return <Navigate to="/dashboard" replace />;

  const authError = searchParams.get("auth_error");
  const notice: LandingNotice | null =
    authError === "oauth_cancelled" ? "oauth_cancelled" : null;

  function onNoticeDismiss() {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.delete("auth_error");
      return next;
    }, { replace: true });
  }

  async function onAuthenticated(session: { csrfToken: string }) {
    await markAuthenticated(session);
    navigate("/dashboard", { replace: true });
  }

  return (
    <LandingPage
      onAuthenticated={(session) => void onAuthenticated(session)}
      notice={notice}
      onNoticeDismiss={onNoticeDismiss}
    />
  );
}

// --------------------------------------------------------------------------
// Dashboard route — protected
// --------------------------------------------------------------------------

function DashboardRoute() {
  const { status } = useAuth();

  if (status === "loading") return <LoadingScreen />;
  if (status === "anonymous") return <Navigate to="/" replace />;

  return <DashboardPage />;
}

// --------------------------------------------------------------------------
// Root router
// --------------------------------------------------------------------------

export function AppRouter() {
  return (
    <BrowserRouter>
      <TitleManager />
      <Routes>
        <Route path="/" element={<LandingRoute />} />
        <Route path="/dashboard" element={<DashboardRoute />} />
        {MOCK_ACTIVE && (
          <Route path="/__mock/oauth/google" element={<MockOAuthPage />} />
        )}
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </BrowserRouter>
  );
}
