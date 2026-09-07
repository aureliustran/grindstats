import { useTranslation } from "react-i18next";
import { mergeRefs, useReveal, useScaledEmbed, useSpotlight } from "./effects";

const PIN_WIDTH = 345;
const PIN_HEIGHT = 714;
const PIN_ID = "941041284670461441";

export function Quote() {
  const { t } = useTranslation();
  const [embedRevealRef, embedVisible] = useReveal<HTMLDivElement>();
  const [quoteRevealRef, quoteVisible] = useReveal<HTMLQuoteElement>();
  const [wrapRef, boxRef] = useScaledEmbed<HTMLDivElement, HTMLDivElement>(
    PIN_WIDTH,
    PIN_HEIGHT,
  );
  const spotlightRef = useSpotlight<HTMLDivElement>();

  return (
    <section className="py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="grid items-center gap-10 md:grid-cols-2">
          {/* IMAGE SLOT 5: Pinterest embed with responsive auto-scaling */}
          <div
            ref={mergeRefs(embedRevealRef, wrapRef)}
            className={`reveal pin-embed-wrap mx-auto ${embedVisible ? "is-visible" : ""}`}
            data-slot="portrait"
          >
            <div
              ref={mergeRefs(boxRef, spotlightRef)}
              className="pin-embed spotlight glass-card overflow-hidden rounded-3xl"
              data-scale-embed
            >
              <iframe
                src={`https://assets.pinterest.com/ext/embed.html?id=${PIN_ID}`}
                width={PIN_WIDTH}
                height={PIN_HEIGHT}
                frameBorder="0"
                scrolling="no"
                title={t("landing.quote.pin_title")}
                loading="lazy"
              />
            </div>
          </div>

          <blockquote
            ref={quoteRevealRef}
            className={`reveal m-0 ${quoteVisible ? "is-visible" : ""}`}
          >
            <p className="font-display text-4xl font-black uppercase leading-none text-ink-0 md:text-6xl">
              "{t("landing.quote.line1")}
              <br />
              {t("landing.quote.line2_pre")}
              <span className="text-accent">
                {t("landing.quote.line2_emphasis")}
              </span>
              {t("landing.quote.line2_post")}"
            </p>
            <footer className="mt-6 flex items-center gap-3 text-sm text-ink-2">
              <span
                className="img-slot h-10 w-10 rounded-full inline-block flex-none"
                data-slot="avatar"
                aria-hidden="true"
              />
              — {t("landing.quote.attribution")}
            </footer>
          </blockquote>
        </div>
      </div>
    </section>
  );
}
