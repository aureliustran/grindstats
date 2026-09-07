import { useTranslation } from "react-i18next";
import { BrainCircuit, Camera, Dumbbell, Ruler, Scale } from "lucide-react";
import { mergeRefs, useReveal, useSpotlight } from "./effects";

interface FeatureItem {
  icon: typeof Scale;
  titleKey: string;
  bodyKey: string;
  metaKey: string;
  delay: string;
}

const ITEMS: FeatureItem[] = [
  {
    icon: Scale,
    titleKey: "metrics_title",
    bodyKey: "metrics_body",
    metaKey: "metrics_meta",
    delay: "0ms",
  },
  {
    icon: Dumbbell,
    titleKey: "training_title",
    bodyKey: "training_body",
    metaKey: "training_meta",
    delay: "80ms",
  },
  {
    icon: Camera,
    titleKey: "nutrition_title",
    bodyKey: "nutrition_body",
    metaKey: "nutrition_meta",
    delay: "160ms",
  },
  {
    icon: BrainCircuit,
    titleKey: "coaching_title",
    bodyKey: "coaching_body",
    metaKey: "coaching_meta",
    delay: "240ms",
  },
];

function FeatureCard({ item }: { item: FeatureItem }) {
  const { t } = useTranslation();
  const [revealRef, visible] = useReveal<HTMLElement>();
  const spotlightRef = useSpotlight<HTMLElement>();
  const Icon = item.icon;

  return (
    <article
      ref={mergeRefs(revealRef, spotlightRef)}
      className={`reveal glass-card spotlight feature rounded-2xl p-6 ${visible ? "is-visible" : ""}`}
      style={{ "--d": item.delay } as React.CSSProperties}
    >
      <div className="feature__icon">
        <Icon aria-hidden="true" className="h-6 w-6" />
      </div>
      <h3>{t(`landing.features.${item.titleKey}`)}</h3>
      <p>{t(`landing.features.${item.bodyKey}`)}</p>
      <p className="feature__meta">
        {t(`landing.features.${item.metaKey}`)}
      </p>
    </article>
  );
}

export function Features() {
  const { t } = useTranslation();
  const [headerRef, headerVisible] = useReveal<HTMLDivElement>();

  return (
    <section id="features" className="scroll-mt-28 py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div
          ref={headerRef}
          className={`reveal max-w-2xl ${headerVisible ? "is-visible" : ""}`}
        >
          <p className="eyebrow">
            <Ruler aria-hidden="true" className="h-3.5 w-3.5" />
            {t("landing.features.eyebrow")}
          </p>
          <h2 className="section-title">
            {t("landing.features.title_pre")}
            <span className="text-accent">
              {t("landing.features.title_accent")}
            </span>
          </h2>
          <p className="mt-4 text-ink-2">
            {t("landing.features.subhead")}
          </p>
        </div>

        <div className="mt-12 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {ITEMS.map((item) => (
            <FeatureCard key={item.titleKey} item={item} />
          ))}
        </div>
      </div>
    </section>
  );
}
