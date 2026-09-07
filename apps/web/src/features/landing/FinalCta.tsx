import { useTranslation } from "react-i18next";
import { ArrowRight } from "lucide-react";
import { useMagnetic, useReveal } from "./effects";

type ModalTab = "login" | "signup";

interface FinalCtaProps {
  onOpenModal: (tab: ModalTab, opener: HTMLButtonElement) => void;
}

export function FinalCta({ onOpenModal }: FinalCtaProps) {
  const { t } = useTranslation();
  const [ref, visible] = useReveal<HTMLDivElement>();
  const magneticRef = useMagnetic<HTMLButtonElement>();

  return (
    <section className="pb-24">
      <div className="mx-auto max-w-6xl px-6">
        <div
          ref={ref}
          className={`reveal relative overflow-hidden rounded-3xl bg-ink-0 p-10 text-paper-1 md:p-16 ${visible ? "is-visible" : ""}`}
        >
          <div className="absolute inset-0 opacity-30 mix-blend-screen">
            <div
              className="img-slot h-full w-full rounded-none"
              data-slot="cta-texture"
            >
              <span className="img-slot__label">
                {t("landing.cta.slot_texture")}
              </span>
            </div>
          </div>
          <div className="relative">
            <p className="font-display text-5xl font-black uppercase leading-none md:text-7xl">
              {t("landing.cta.heading_line1")}
              <br />
              <span className="text-accent">
                {t("landing.cta.heading_line2")}
              </span>
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
              <button
                ref={magneticRef}
                type="button"
                onClick={(e) => onOpenModal("signup", e.currentTarget)}
                className="btn-primary btn-lg magnetic"
                data-open-auth="signup"
              >
                {t("landing.hero.cta_primary")}{" "}
                <ArrowRight aria-hidden="true" className="h-4 w-4" />
              </button>
              <button
                type="button"
                onClick={(e) => onOpenModal("login", e.currentTarget)}
                className="btn-ghost btn-lg !border-paper-1/30 !text-paper-1"
                data-open-auth="login"
              >
                {t("landing.cta.secondary")}
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
