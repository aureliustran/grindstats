/* Landing motion hooks matching template/app.js.
   Every effect respects prefers-reduced-motion: reduce. */

import { useEffect, useRef, useState, type RefObject } from "react";

/** Combines multiple refs onto one DOM node. */
export function mergeRefs<T>(...refs: Array<RefObject<T> | undefined>) {
  return (node: T | null) => {
    for (const ref of refs) {
      if (ref) (ref as { current: T | null }).current = node;
    }
  };
}

export const prefersReducedMotion = () =>
  typeof matchMedia === "function" &&
  matchMedia("(prefers-reduced-motion: reduce)").matches;

/** Fades an element in once it crosses the viewport, respecting reduced motion. */
export function useReveal<T extends HTMLElement>(): [RefObject<T>, boolean] {
  const ref = useRef<T>(null);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (prefersReducedMotion()) {
      setVisible(true);
      el.classList.add("is-visible", "u-is-visible");
      return;
    }
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setVisible(true);
          el.classList.add("is-visible", "u-is-visible");
          io.disconnect();
        }
      },
      { threshold: 0.15 },
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  return [ref, visible];
}

/** Counts a number up from 0 once `active`, formatted with active locale. */
export function useCountUp(
  target: number,
  active: boolean,
  locale: string,
  decimals = 0,
): string {
  const [value, setValue] = useState(0);
  const done = useRef(false);

  useEffect(() => {
    if (!active || done.current) return;
    done.current = true;
    if (prefersReducedMotion()) {
      setValue(target);
      return;
    }
    const start = performance.now();
    const duration = 1400;
    let frame: number;
    const step = (now: number) => {
      const p = Math.min(1, (now - start) / duration);
      const eased = 1 - Math.pow(1 - p, 3);
      setValue(target * eased);
      if (p < 1) frame = requestAnimationFrame(step);
    };
    frame = requestAnimationFrame(step);
    return () => cancelAnimationFrame(frame);
  }, [active, target]);

  return new Intl.NumberFormat(locale, {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  }).format(value);
}

/** Sets --mx/--my custom properties on pointermove for .spotlight. */
export function useSpotlight<T extends HTMLElement>(): RefObject<T> {
  const ref = useRef<T>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const onMove = (e: PointerEvent) => {
      const r = el.getBoundingClientRect();
      el.style.setProperty("--mx", `${((e.clientX - r.left) / r.width) * 100}%`);
      el.style.setProperty("--my", `${((e.clientY - r.top) / r.height) * 100}%`);
    };
    el.addEventListener("pointermove", onMove);
    return () => el.removeEventListener("pointermove", onMove);
  }, []);
  return ref;
}

/** Subtle 3D tilt toward the pointer. No-op under reduced motion. */
export function useTilt<T extends HTMLElement>(): RefObject<T> {
  const ref = useRef<T>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el || prefersReducedMotion()) return;
    const onMove = (e: PointerEvent) => {
      const r = el.getBoundingClientRect();
      const x = (e.clientX - r.left) / r.width - 0.5;
      const y = (e.clientY - r.top) / r.height - 0.5;
      el.style.transform = `perspective(900px) rotateX(${(-y * 8).toFixed(2)}deg) rotateY(${(x * 10).toFixed(2)}deg) translateZ(0)`;
    };
    const onLeave = () => { el.style.transform = ""; };
    el.addEventListener("pointermove", onMove);
    el.addEventListener("pointerleave", onLeave);
    return () => {
      el.removeEventListener("pointermove", onMove);
      el.removeEventListener("pointerleave", onLeave);
    };
  }, []);
  return ref;
}

/** Nudges an element toward the pointer. No-op under reduced motion. */
export function useMagnetic<T extends HTMLElement>(): RefObject<T> {
  const ref = useRef<T>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el || prefersReducedMotion()) return;
    const onMove = (e: PointerEvent) => {
      const r = el.getBoundingClientRect();
      const x = (e.clientX - r.left - r.width / 2) * 0.18;
      const y = (e.clientY - r.top - r.height / 2) * 0.25;
      el.style.transform = `translate(${x.toFixed(1)}px, ${y.toFixed(1)}px)`;
    };
    const onLeave = () => { el.style.transform = ""; };
    el.addEventListener("pointermove", onMove);
    el.addEventListener("pointerleave", onLeave);
    return () => {
      el.removeEventListener("pointermove", onMove);
      el.removeEventListener("pointerleave", onLeave);
    };
  }, []);
  return ref;
}

/**
 * Scales a fixed-size embed (the Pinterest iframe) down to fit its wrapper's
 * actual width, adjusting the wrapper's height to match.
 */
export function useScaledEmbed<W extends HTMLElement, B extends HTMLElement>(
  nativeWidth: number,
  nativeHeight: number,
): [RefObject<W>, RefObject<B>] {
  const wrapRef = useRef<W>(null);
  const boxRef = useRef<B>(null);

  useEffect(() => {
    const wrap = wrapRef.current;
    const box = boxRef.current;
    if (!wrap || !box) return;

    const apply = () => {
      const scale = Math.min(1, wrap.clientWidth / nativeWidth);
      box.style.transform = `scale(${scale})`;
      wrap.style.height = `${nativeHeight * scale}px`;
    };

    apply();
    const ro = new ResizeObserver(apply);
    ro.observe(wrap);
    window.addEventListener("resize", apply);
    const raf1 = requestAnimationFrame(() => requestAnimationFrame(apply));

    return () => {
      ro.disconnect();
      window.removeEventListener("resize", apply);
      cancelAnimationFrame(raf1);
    };
  }, [nativeWidth, nativeHeight]);

  return [wrapRef, boxRef];
}

/** Scroll spy: observes section elements and returns the active section ID */
export function useScrollSpy(sectionIds: string[]): string {
  const [activeId, setActiveId] = useState<string>("");

  useEffect(() => {
    const elements = sectionIds
      .map((id) => document.getElementById(id))
      .filter((el): el is HTMLElement => el !== null);

    if (elements.length === 0) return;

    const io = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            setActiveId(entry.target.id);
          }
        });
      },
      { rootMargin: "-40% 0px -55% 0px" },
    );

    elements.forEach((el) => io.observe(el));
    return () => io.disconnect();
  }, [sectionIds]);

  return activeId;
}
