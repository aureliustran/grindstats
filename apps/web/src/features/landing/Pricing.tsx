import { useTranslation } from "react-i18next";
import { Check, Minus, Sparkles, Tag } from "lucide-react";
import { mergeRefs, useReveal, useSpotlight } from "./effects";

type ModalTab = "login" | "signup";

interface PricingProps {
  onOpenModal: (tab: ModalTab, opener: HTMLButtonElement) => void;
}

export function Pricing({ onOpenModal }: PricingProps) {
  const { t } = useTranslation();
  const [headerRef, headerVisible] = useReveal<HTMLDivElement>();
  const [freeRef, freeVisible] = useReveal<HTMLElement>();
  const [proRef, proVisible] = useReveal<HTMLElement>();
  const [artRef, artVisible] = useReveal<HTMLDivElement>();
  const freeSpotlight = useSpotlight<HTMLElement>();
  const proSpotlight = useSpotlight<HTMLElement>();

  const handleCta = (e: React.MouseEvent<HTMLButtonElement>) => {
    onOpenModal("signup", e.currentTarget);
  };

  return (
    <section id="pricing" className="scroll-mt-28 py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="grid items-center gap-10 lg:grid-cols-[1.1fr_0.9fr]">
          <div>
            <div
              ref={headerRef}
              className={`reveal ${headerVisible ? "is-visible" : ""}`}
            >
              <p className="eyebrow">
                <Tag aria-hidden="true" className="h-3.5 w-3.5" />
                {t("landing.pricing.heading")}
              </p>
              <h2 className="section-title">
                {t("landing.pricing.title")}
              </h2>
              <p className="mt-2 text-ink-2">
                {t("landing.pricing.placeholder")}
              </p>
            </div>

            <div className="mt-10 grid gap-4 sm:grid-cols-2">
              <article
                ref={mergeRefs(freeRef, freeSpotlight)}
                className={`reveal glass-card spotlight price rounded-3xl p-8 ${freeVisible ? "is-visible" : ""}`}
              >
                <h3>{t("landing.pricing.free_title")}</h3>
                <p className="price__amt">
                  <span className="font-mono">0</span>
                  <small>{t("landing.pricing.free_period")}</small>
                </p>
                <ul className="list-none p-0">
                  <li>
                    <Check aria-hidden="true" className="h-4 w-4" />
                    {t("landing.pricing.free_feat_1")}
                  </li>
                  <li>
                    <Check aria-hidden="true" className="h-4 w-4" />
                    {t("landing.pricing.free_feat_2")}
                  </li>
                  <li>
                    <Check aria-hidden="true" className="h-4 w-4" />
                    {t("landing.pricing.free_feat_3")}
                  </li>
                  <li className="muted">
                    <Minus aria-hidden="true" className="h-4 w-4" />
                    {t("landing.pricing.free_feat_4_muted")}
                  </li>
                  <li className="muted">
                    <Minus aria-hidden="true" className="h-4 w-4" />
                    {t("landing.pricing.free_feat_5_muted")}
                  </li>
                </ul>
                <button
                  type="button"
                  onClick={handleCta}
                  className="btn-ghost mt-6 w-full"
                  data-open-auth="signup"
                >
                  {t("landing.pricing.cta")}
                </button>
              </article>

              <article
                ref={mergeRefs(proRef, proSpotlight)}
                className={`reveal glass-card spotlight price price--featured rounded-3xl p-8 relative ${proVisible ? "is-visible" : ""}`}
                style={{ "--d": "120ms" } as React.CSSProperties}
              >
                <span className="badge absolute right-6 top-6">
                  <Sparkles aria-hidden="true" className="h-3.5 w-3.5" />
                  {t("landing.pricing.pro_badge")}
                </span>
                <h3>{t("landing.pricing.pro_title")}</h3>
                <p className="price__amt">
                  <span className="font-mono">—</span>
                  <small>{t("landing.pricing.pro_price_note")}</small>
                </p>
                <ul className="list-none p-0">
                  <li>
                    <Check aria-hidden="true" className="h-4 w-4" />
                    {t("landing.pricing.pro_feat_1")}
                  </li>
                  <li>
                    <Check aria-hidden="true" className="h-4 w-4" />
                    {t("landing.pricing.pro_feat_2")}
                  </li>
                  <li>
                    <Check aria-hidden="true" className="h-4 w-4" />
                    {t("landing.pricing.pro_feat_3")}
                  </li>
                  <li>
                    <Check aria-hidden="true" className="h-4 w-4" />
                    {t("landing.pricing.pro_feat_4")}
                  </li>
                  <li>
                    <Check aria-hidden="true" className="h-4 w-4" />
                    {t("landing.pricing.pro_feat_5")}
                  </li>
                </ul>
                <button
                  type="button"
                  onClick={handleCta}
                  className="btn-primary mt-6 w-full"
                  data-open-auth="signup"
                >
                  {t("landing.pricing.pro_cta")}
                </button>
              </article>
            </div>
          </div>

          {/* IMAGE SLOT 7: pricing side art */}
          <div
            ref={artRef}
            className={`reveal relative ${artVisible ? "is-visible" : ""}`}
            style={{ "--d": "60ms" } as React.CSSProperties}
          >
            <div
              className="absolute -inset-8 -z-10 rounded-[2rem] bg-accent/15 blur-3xl"
              aria-hidden="true"
            />
            <div
              className="img-slot pricing-art relative aspect-[3/4] overflow-hidden rounded-3xl"
              data-slot="pricing-side"
            >
              <img
                src="/media/download.jpg"
                alt=""
                aria-hidden="true"
              />
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
