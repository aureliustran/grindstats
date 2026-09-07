/* =============================================================================
   Root component — mounts the auth provider, error boundary, and router.
   ============================================================================= */

import { ErrorBoundary } from "./app/ErrorBoundary";
import { AuthProvider } from "./app/AuthContext";
import { AppRouter } from "./app/Router";

export default function App() {
  return (
    <ErrorBoundary>
      <AuthProvider>
        <AppRouter />
      </AuthProvider>
    </ErrorBoundary>
  );
}
