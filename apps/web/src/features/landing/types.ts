export type LandingNotice = "oauth_cancelled";

export interface LandingPageProps {
  onAuthenticated: (session: { csrfToken: string }) => void;
  notice: LandingNotice | null;
  onNoticeDismiss: () => void;
}
