import { useTranslation } from "react-i18next";
import { Calculator } from "lucide-react";
import { formatNumber } from "../../i18n";
import { useCountUp, useReveal } from "./effects";

const MEAN_INTAKE = 2400;
const WEIGHT_TREND = 0.8;
const FROM_STORES = 6160;
const ADAPTIVE_TDEE = 2620;
const CI_LOW = 2396;
const CI_HIGH = 2844;

export function Numbers() {
  const { t, i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? "en-US";
  const [cardRef, cardVisible] = useReveal<HTMLDivElement>();

  const meanIntake = useCountUp(MEAN_INTAKE, cardVisible, locale);
  const weightTrend = useCountUp(WEIGHT_TREND, cardVisible, locale, 1);
  const fromStores = useCountUp(FROM_STORES, cardVisible, locale);
  const adaptiveTdee = useCountUp(ADAPTIVE_TDEE, cardVisible, locale);

  return (
    <section id="numbers" className="scroll-mt-28 py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div
          ref={cardRef}
          className={`glass-card reveal relative overflow-hidden rounded-3xl p-8 md:p-12 ${cardVisible ? "is-visible" : ""}`}
        >
          <div
            className="absolute -right-24 -top-24 h-72 w-72 rounded-full bg-accent/25 blur-3xl"
            aria-hidden="true"
          />
          <div className="grid gap-10 md:grid-cols-[1fr_1.2fr] md:items-center">
            <div>
              <p className="eyebrow">
                <Calculator aria-hidden="true" className="h-3.5 w-3.5" />
                {t("landing.numbers.eyebrow")}
              </p>
              <h2 className="section-title">
                {t("landing.numbers.heading_line1")}
                <br />
                <span className="text-ink-3">
                  {t("landing.numbers.heading_line2")}
                </span>
              </h2>
              <p className="mt-4 text-ink-2">
                {t("landing.numbers.body")}
              </p>
            </div>

            <dl className="grid grid-cols-2 gap-4 m-0">
              <div className="stat">
                <dt>{t("landing.numbers.stat_mean_intake")}</dt>
                <dd>
                  <span>{meanIntake}</span>
                  <small>{t("landing.numbers.unit_kcal_per_day")}</small>
                </dd>
              </div>
              <div className="stat">
                <dt>{t("landing.numbers.stat_weight_trend")}</dt>
                <dd>
                  −<span>{weightTrend}</span>
                  <small>{t("landing.numbers.unit_kg_per_28d")}</small>
                </dd>
              </div>
              <div className="stat">
                <dt>{t("landing.numbers.stat_from_stores")}</dt>
                <dd>
                  <span>{fromStores}</span>
                  <small>{t("landing.numbers.unit_kcal")}</small>
                </dd>
              </div>
              <div className="stat stat--accent">
                <dt>{t("landing.numbers.stat_adaptive_tdee")}</dt>
                <dd>
                  <span>{adaptiveTdee}</span>
                  <small>{t("landing.numbers.unit_kcal_per_day")}</small>
                </dd>
                <p className="estimated mt-1 text-xs">
                  {t("landing.numbers.ci_note", {
                    low: formatNumber(CI_LOW),
                    high: formatNumber(CI_HIGH),
                  })}
                </p>
              </div>
            </dl>
          </div>
        </div>
      </div>
    </section>
  );
}
