import { useTranslation } from "react-i18next";
import { Route } from "lucide-react";
import { useReveal } from "./effects";

interface StepItem {
  num: string;
  slot: string;
  labelKey: string;
  titleKey: string;
  bodyKey: string;
  delay?: string;
  customBody?: boolean;
}

const STEPS: StepItem[] = [
  {
    num: "01",
    slot: "screen-checklist",
    labelKey: "slot_checklist",
    titleKey: "step_log_title",
    bodyKey: "step_log_body",
  },
  {
    num: "02",
    slot: "screen-trend",
    labelKey: "slot_trend",
    titleKey: "step_measure_title",
    bodyKey: "step_measure_body",
    delay: "120ms",
    customBody: true,
  },
  {
    num: "03",
    slot: "screen-coach",
    labelKey: "slot_coach",
    titleKey: "step_adapt_title",
    bodyKey: "step_adapt_body",
    delay: "240ms",
  },
];

function StepCard({ step }: { step: StepItem }) {
  const { t } = useTranslation();
  const [ref, visible] = useReveal<HTMLLIElement>();

  return (
    <li
      ref={ref}
      className={`reveal step ${visible ? "is-visible" : ""}`}
      style={step.delay ? ({ "--d": step.delay } as React.CSSProperties) : undefined}
    >
      <div className="img-slot step__shot" data-slot={step.slot}>
        <span className="img-slot__label">
          {t(`landing.how_it_works.${step.labelKey}`)}
        </span>
      </div>
      <div className="step__num font-mono">{step.num}</div>
      <h3>{t(`landing.how_it_works.${step.titleKey}`)}</h3>
      {step.customBody ? (
        <p>
          {t("landing.how_it_works.step_measure_body_pre")}{" "}
          <span className="font-mono text-ink-0">21</span>{" "}
          {t("landing.how_it_works.step_measure_body_post")}
        </p>
      ) : (
        <p>{t(`landing.how_it_works.${step.bodyKey}`)}</p>
      )}
    </li>
  );
}

export function HowItWorks() {
  const { t } = useTranslation();
  const [headerRef, headerVisible] = useReveal<HTMLDivElement>();

  return (
    <section id="how" className="scroll-mt-28 py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div
          ref={headerRef}
          className={`reveal max-w-2xl ${headerVisible ? "is-visible" : ""}`}
        >
          <p className="eyebrow">
            <Route aria-hidden="true" className="h-3.5 w-3.5" />
            {t("landing.how_it_works.eyebrow")}
          </p>
          <h2 className="section-title">
            {t("landing.how_it_works.title_log")}{" "}
            <span className="text-ink-3">
              {t("landing.how_it_works.title_accumulate")}
            </span>{" "}
            {t("landing.how_it_works.title_adapt")}
          </h2>
        </div>

        <ol className="mt-12 grid gap-6 lg:grid-cols-3 list-none p-0">
          {STEPS.map((step) => (
            <StepCard key={step.num} step={step} />
          ))}
        </ol>
      </div>
    </section>
  );
}
