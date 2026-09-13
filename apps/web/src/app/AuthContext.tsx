/* =============================================================================
   AuthProvider + useAuth hook.

   State machine:
     "loading"       — boot probe in flight; shell renders common.loading
     "anonymous"     — me() returned null; show landing page
     "authenticated" — me() returned a user; redirect to /dashboard

   Nothing outside src/app/ should import the context object — only useAuth().
   ============================================================================= */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { authApi } from "../api/auth";
import type { CurrentUser } from "../api/auth.types";

// --------------------------------------------------------------------------
// State shape
// --------------------------------------------------------------------------

export type AuthStatus = "loading" | "anonymous" | "authenticated";

export interface AuthState {
  status: AuthStatus;
  user?: CurrentUser;
  csrfToken?: string;
  emailVerified: boolean;
}

export interface AuthContextValue extends AuthState {
  /**
   * Called by the landing page after a successful login.
   * Stores the CSRF token, then re-probes me() to populate the user.
   */
  markAuthenticated: (session: { csrfToken: string }) => Promise<void>;
  /** Log the current user out and return to anonymous state. */
  logout: () => Promise<void>;
}

// --------------------------------------------------------------------------
// Context
// --------------------------------------------------------------------------

const AuthContext = createContext<AuthContextValue | null>(null);

// --------------------------------------------------------------------------
// Provider
// --------------------------------------------------------------------------

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: "loading", emailVerified: false });

  // Boot probe — runs once on mount
  useEffect(() => {
    let cancelled = false;
    authApi
      .me()
      .then((session) => {
        if (cancelled) return;
        if (session) {
          setState({
            status: "authenticated",
            user: session.user,
            csrfToken: session.csrf_token,
            emailVerified: session.user.email_verified ?? true,
          });
        } else {
          setState({ status: "anonymous", emailVerified: false });
        }
      })
      .catch(() => {
        if (cancelled) return;
        // Network failure on boot — treat as anonymous so the app is usable
        setState({ status: "anonymous", emailVerified: false });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const markAuthenticated = useCallback(
    async (session: { csrfToken: string }) => {
      setState((prev) => ({ ...prev, status: "loading" }));
      try {
        const probe = await authApi.me();
        if (probe) {
          setState({
            status: "authenticated",
            user: probe.user,
            csrfToken: session.csrfToken || probe.csrf_token,
            emailVerified: probe.user.email_verified ?? true,
          });
        } else {
          setState({ status: "anonymous", emailVerified: false });
        }
      } catch {
        setState({ status: "anonymous", emailVerified: false });
      }
    },
    [],
  );

  const logout = useCallback(async () => {
    const csrfToken = state.csrfToken ?? "";
    setState((prev) => ({ ...prev, status: "loading" }));
    try {
      await authApi.logout(csrfToken);
    } catch {
      // Best-effort: clear local state regardless
    }
    setState({ status: "anonymous", emailVerified: false });
  }, [state.csrfToken]);

  return (
    <AuthContext.Provider value={{ ...state, markAuthenticated, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

// --------------------------------------------------------------------------
// Hook
// --------------------------------------------------------------------------

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used inside <AuthProvider>");
  }
  return ctx;
}
