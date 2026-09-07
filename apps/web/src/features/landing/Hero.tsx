import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ArrowRight,
  CalendarCheck,
  Camera,
  CheckSquare,
  Flame,
  Play,
  ShieldCheck,
  Sigma,
  Square,
} from "lucide-react";
import { formatNumber } from "../../i18n";
import {
  mergeRefs,
  prefersReducedMotion,
  useCountUp,
  useMagnetic,
  useReveal,
  useSpotlight,
  useTilt,
} from "./effects";

type ModalTab = "login" | "signup";

interface HeroProps {
  onOpenModal: (tab: ModalTab, opener: HTMLButtonElement) => void;
}

const STREAK_DAYS = 214;
const ENERGY_KCAL = 1842;

export function Hero({ onOpenModal }: HeroProps) {
  const { t, i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? "en-US";

  const [leftRef, leftVisible] = useReveal<HTMLDivElement>();
  const [cardRevealRef, cardVisible] = useReveal<HTMLDivElement>();
  const cardSpotlightRef = useSpotlight<HTMLDivElement>();
  const cardTiltRef = useTilt<HTMLDivElement>();
  const magneticRef = useMagnetic<HTMLButtonElement>();

  const [videoFallback, setVideoFallback] = useState(false);
  const videoRef = useRef<HTMLVideoElement>(null);
  const parallaxRef = useRef<HTMLDivElement>(null);

  const streak = useCountUp(STREAK_DAYS, cardVisible, locale);
  const energy = useCountUp(ENERGY_KCAL, cardVisible, locale);

  // Video playback & parallax
  useEffect(() => {
    const video = videoRef.current;
    if (video && prefersReducedMotion()) {
      video.pause();
    }

    const parallaxEl = parallaxRef.current;
    if (!parallaxEl || prefersReducedMotion()) return;

    const onScroll = () => {
      parallaxEl.style.transform = `translateY(${window.scrollY * 0.15}px)`;
    };

    window.addEventListener("scroll", onScroll, { passive: true });
    onScroll();

    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  const handleStartTracking = (e: React.MouseEvent<HTMLButtonElement>) => {
    onOpenModal("signup", e.currentTarget);
  };

  return (
    <section className="relative min-h-screen overflow-hidden pt-36 pb-24">
      {/* Video Backdrop */}
      <div
        ref={parallaxRef}
        className={`img-slot hero-bg absolute inset-0 -z-10 ${videoFallback ? "hero-bg--fallback" : ""}`}
        data-slot="hero-backdrop"
      >
        <video
          ref={videoRef}
          className="hero-bg__video"
          src="/media/saitama-vs-garou.mp4"
          autoPlay
          loop
          muted
          playsInline
          aria-hidden="true"
          onError={() => setVideoFallback(true)}
        />
      </div>

      {/* Fade to paper-1 at bottom */}
      <div
        className="absolute inset-0 -z-10"
        style={{
          background:
            "linear-gradient(180deg, transparent 0%, transparent 78%, var(--paper-1) 100%)",
        }}
        aria-hidden="true"
      />

      <div className="mx-auto grid max-w-6xl grid-cols-1 items-center gap-10 px-6 text-white lg:grid-cols-[1.2fr_1fr]">
        {/* Left Column: Copy & CTAs */}
        <div ref={leftRef} className={`reveal ${leftVisible ? "is-visible" : ""}`}>
          <p className="eyebrow">
            <CalendarCheck aria-hidden="true" className="h-3.5 w-3.5" />
            {t("landing.hero.eyebrow")}
          </p>

          <h1 className="mt-3 font-display text-[clamp(3rem,9vw,7.5rem)] font-black uppercase leading-[0.9] tracking-tight text-white">
            <span className="block">{t("landing.hero.regimen_pushups")}</span>
            <span className="block">{t("landing.hero.regimen_situps")}</span>
            <span className="block">{t("landing.hero.regimen_squats")}</span>
            <span className="block">{t("landing.hero.regimen_run")}</span>
            <span className="block text-accent glow-text">
              {t("landing.hero.punchline")}
            </span>
          </h1>

          <p className="mt-4 max-w-md font-mono text-sm italic text-white/60">
            {t("landing.hero.aside")}
          </p>
          <p className="mt-6 max-w-lg text-lg text-white/85">
            {t("landing.hero.subhead")}
          </p>

          <div className="mt-8 flex flex-wrap items-center gap-3">
            <button
              ref={magneticRef}
              type="button"
              onClick={handleStartTracking}
              className="btn-primary btn-lg magnetic"
              data-open-auth="signup"
            >
              {t("landing.hero.cta_primary")}{" "}
              <ArrowRight aria-hidden="true" className="h-4 w-4" />
            </button>
            <a href="#how" className="btn-ghost btn-lg">
              <Play aria-hidden="true" className="h-4 w-4" />{" "}
              {t("landing.hero.cta_secondary")}
            </a>
          </div>

          <ul className="mt-10 flex flex-wrap gap-x-6 gap-y-2 text-sm text-white/70 list-none p-0">
            <li className="flex items-center gap-2">
              <ShieldCheck aria-hidden="true" className="h-4 w-4 text-accent" />
              {t("landing.hero.bullet_no_hype")}
            </li>
            <li className="flex items-center gap-2">
              <Sigma aria-hidden="true" className="h-4 w-4 text-accent" />
              {t("landing.hero.bullet_numbers")}
            </li>
            <li className="flex items-center gap-2">
              <Camera aria-hidden="true" className="h-4 w-4 text-accent" />
              {t("landing.hero.bullet_photo")}
            </li>
          </ul>
        </div>

        {/* Right Column: Glass "live checklist" card */}
        <div className="relative">
          <div
            className="absolute -inset-6 -z-10 rounded-[2rem] bg-accent/20 blur-3xl"
            aria-hidden="true"
          />
          <div
            ref={mergeRefs(cardRevealRef, cardSpotlightRef, cardTiltRef)}
            className={`glass-card spotlight tilt rounded-3xl p-6 reveal ${cardVisible ? "is-visible" : ""}`}
            data-tilt
          >
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs uppercase tracking-widest text-ink-2">
                  {t("landing.hero.card_occurrence", { n: formatNumber(STREAK_DAYS) })}
                </p>
                <h3 className="font-display text-2xl font-black uppercase text-ink-0">
                  {t("landing.hero.card_title")}
                </h3>
              </div>
              <span className="badge">
                <Flame aria-hidden="true" className="h-3.5 w-3.5" />
                <span className="font-mono">{t("landing.hero.card_streak", { streak })}</span>
              </span>
            </div>

            <ul className="mt-5 space-y-2 list-none p-0">
              <li className="check-row is-done">
                <CheckSquare aria-hidden="true" className="h-5 w-5" />
                <span>{t("landing.hero.regimen_pushups")}</span>
                <span className="ml-auto font-mono text-xs text-ink-2">
                  {t("landing.hero.card_reps_pushups")}
                </span>
              </li>
              <li className="check-row is-done">
                <CheckSquare aria-hidden="true" className="h-5 w-5" />
                <span>{t("landing.hero.regimen_situps")}</span>
                <span className="ml-auto font-mono text-xs text-ink-2">
                  {t("landing.hero.card_reps_situps")}
                </span>
              </li>
              <li className="check-row is-active">
                <Square aria-hidden="true" className="h-5 w-5" />
                <span>{t("landing.hero.regimen_squats")}</span>
                <span className="ml-auto font-mono text-xs text-accent">
                  {t("landing.hero.card_status_in_progress")}
                </span>
              </li>
              <li className="check-row">
                <Square aria-hidden="true" className="h-5 w-5" />
                <span>{t("landing.hero.regimen_run")}</span>
                <span className="ml-auto font-mono text-xs text-ink-2">
                  {t("landing.hero.card_reps_run")}
                </span>
              </li>
            </ul>

            <div className="mt-5 rounded-xl border border-paper-3/40 bg-paper-0/40 p-4">
              <div className="flex items-end justify-between">
                <div>
                  <p className="text-xs text-ink-2">
                    {t("landing.hero.card_energy_label")}
                  </p>
                  <p className="font-mono text-3xl font-semibold text-ink-0">
                    <span>{energy}</span>{" "}
                    <span className="text-base text-ink-2">
                      {t("landing.hero.card_energy_unit")}
                    </span>
                  </p>
                </div>
                <p className="estimated text-xs">
                  {t("landing.hero.card_estimated")}
                </p>
              </div>
              <div className="mt-3 h-2 overflow-hidden rounded-full bg-paper-3/50">
                <div
                  className="progress h-full rounded-full bg-accent"
                  style={{ "--to": "68%" } as React.CSSProperties}
                />
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Marquee ticker */}
      <div
        className="ticker mt-20 border-y border-paper-3/40 bg-paper-0/30 py-3 backdrop-blur"
        aria-hidden="true"
      >
        <div className="ticker__track font-display text-2xl font-black uppercase tracking-wide text-ink-2">
          <span>{t("landing.hero.regimen_pushups")}</span>
          <span className="dot" />
          <span>{t("landing.hero.regimen_situps")}</span>
          <span className="dot" />
          <span>{t("landing.hero.regimen_squats")}</span>
          <span className="dot" />
          <span>{t("landing.hero.regimen_run")}</span>
          <span className="dot" />
          <span className="text-accent">{t("landing.hero.punchline")}</span>
          <span className="dot" />
          <span>{t("landing.hero.regimen_pushups")}</span>
          <span className="dot" />
          <span>{t("landing.hero.regimen_situps")}</span>
          <span className="dot" />
          <span>{t("landing.hero.regimen_squats")}</span>
          <span className="dot" />
          <span>{t("landing.hero.regimen_run")}</span>
          <span className="dot" />
          <span className="text-accent">{t("landing.hero.punchline")}</span>
          <span className="dot" />
        </div>
      </div>
    </section>
  );
}
