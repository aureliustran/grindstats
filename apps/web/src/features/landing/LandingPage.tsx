import { useState, useRef, useCallback } from "react";
import { useTranslation } from "react-i18next";
import type { LandingPageProps } from "./types";
import { Header } from "./Header";
import { Hero } from "./Hero";
import { Sections } from "./Sections";
import { Footer } from "./Footer";
import { AuthModal } from "./AuthModal";
import { Notice } from "./Notice";

type ModalTab = "login" | "signup";

export function LandingPage({
  onAuthenticated,
  notice,
  onNoticeDismiss,
}: LandingPageProps) {
  const { t } = useTranslation();
  const [modalOpen, setModalOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<ModalTab>("login");
  const openerRef = useRef<HTMLButtonElement | null>(null);

  const openModal = useCallback(
    (tab: ModalTab, opener: HTMLButtonElement) => {
      openerRef.current = opener;
      setActiveTab(tab);
      setModalOpen(true);
    },
    [],
  );

  const closeModal = useCallback(() => {
    setModalOpen(false);
    openerRef.current?.focus();
    openerRef.current = null;
  }, []);

  return (
    <>
      <div className="grain" aria-hidden="true" />
      <div className="ambient" aria-hidden="true" />

      <a href="#main" className="skip-link">
        {t("landing.nav.skip_to_content")}
      </a>

      {notice === "oauth_cancelled" && (
        <Notice onDismiss={onNoticeDismiss} />
      )}

      <Header onOpenModal={openModal} />

      <main id="main">
        <Hero onOpenModal={openModal} />
        <Sections onOpenModal={openModal} />
      </main>

      <Footer />

      {modalOpen && (
        <AuthModal
          initialTab={activeTab}
          onClose={closeModal}
          onAuthenticated={onAuthenticated}
        />
      )}
    </>
  );
}
