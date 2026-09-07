/* GrindStats static template — interactions. No framework, no build step. */
(() => {
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const $ = (s, r = document) => r.querySelector(s);
  const $$ = (s, r = document) => [...r.querySelectorAll(s)];

  /* Icons */
  if (window.lucide) lucide.createIcons();

  /* Theme */
  const root = document.documentElement;
  const saved = localStorage.getItem("gs-theme");
  if (saved) root.classList.toggle("dark", saved === "dark");
  $("#themeToggle")?.addEventListener("click", () => {
    const dark = root.classList.toggle("dark");
    localStorage.setItem("gs-theme", dark ? "dark" : "light");
  });

  /* Mobile menu */
  const menuBtn = $("#menuBtn"), menu = $("#mobileMenu");
  const setMenu = (open) => {
    menu.classList.toggle("hidden", !open);
    menuBtn.setAttribute("aria-expanded", String(open));
    menuBtn.setAttribute("aria-label", open ? "Close menu" : "Open menu");
  };
  menuBtn?.addEventListener("click", () => setMenu(menu.classList.contains("hidden")));
  $$("#mobileMenu a, #mobileMenu button").forEach((el) => el.addEventListener("click", () => setMenu(false)));

  /* Active nav link on scroll */
  const links = $$(".nav-link[href^='#']");
  const sections = links.map((a) => $(a.getAttribute("href"))).filter(Boolean);
  const navIo = new IntersectionObserver((entries) => {
    entries.forEach((e) => {
      if (!e.isIntersecting) return;
      links.forEach((a) => a.classList.toggle("is-active", a.getAttribute("href") === "#" + e.target.id));
    });
  }, { rootMargin: "-40% 0px -55% 0px" });
  sections.forEach((s) => navIo.observe(s));

  /* Scroll reveal + counters + progress */
  const io = new IntersectionObserver((entries) => {
    entries.forEach((e) => {
      if (!e.isIntersecting) return;
      e.target.classList.add("is-visible");
      $$("[data-count]", e.target).forEach(countUp);
      io.unobserve(e.target);
    });
  }, { threshold: 0.15 });
  $$(".reveal").forEach((el) => io.observe(el));
  /* Safety net: anything already in the viewport reveals even if the observer is slow. */
  setTimeout(() => $$(".reveal:not(.is-visible)").forEach((el) => {
    if (el.getBoundingClientRect().top < innerHeight) { el.classList.add("is-visible"); $$("[data-count]", el).forEach(countUp); io.unobserve(el); }
  }), 1200);

  function countUp(el) {
    if (el.dataset.done) return;
    el.dataset.done = "1";
    const to = parseFloat(el.dataset.count), dec = +(el.dataset.decimals || 0);
    const fmt = new Intl.NumberFormat(document.documentElement.lang || "en-US", { minimumFractionDigits: dec, maximumFractionDigits: dec });
    if (reduce) { el.textContent = fmt.format(to); return; }
    const t0 = performance.now(), dur = 1400;
    const step = (t) => {
      const p = Math.min(1, (t - t0) / dur), ease = 1 - Math.pow(1 - p, 3);
      el.textContent = fmt.format(to * ease);
      if (p < 1) requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  }

  /* Cursor spotlight on glass cards */
  $$(".spotlight").forEach((card) => {
    card.addEventListener("pointermove", (ev) => {
      const r = card.getBoundingClientRect();
      card.style.setProperty("--mx", `${((ev.clientX - r.left) / r.width) * 100}%`);
      card.style.setProperty("--my", `${((ev.clientY - r.top) / r.height) * 100}%`);
    });
  });

  /* 3D tilt on hero card */
  if (!reduce) $$("[data-tilt]").forEach((card) => {
    card.addEventListener("pointermove", (ev) => {
      const r = card.getBoundingClientRect();
      const x = (ev.clientX - r.left) / r.width - 0.5, y = (ev.clientY - r.top) / r.height - 0.5;
      card.style.transform = `perspective(900px) rotateX(${(-y * 8).toFixed(2)}deg) rotateY(${(x * 10).toFixed(2)}deg) translateZ(0)`;
    });
    card.addEventListener("pointerleave", () => (card.style.transform = ""));
  });

  /* Magnetic buttons */
  if (!reduce) $$(".magnetic").forEach((btn) => {
    btn.addEventListener("pointermove", (ev) => {
      const r = btn.getBoundingClientRect();
      btn.style.transform = `translate(${(ev.clientX - r.left - r.width / 2) * 0.18}px, ${(ev.clientY - r.top - r.height / 2) * 0.25}px)`;
    });
    btn.addEventListener("pointerleave", () => (btn.style.transform = ""));
  });

  /* Pinterest embed — the iframe is a fixed 345×714 box (Pinterest's size,
     not ours). CSS `transform: scale()` doesn't shrink the layout box it
     occupies, so without this the wrapper would reserve the full unscaled
     714px of height even when displayed smaller. This measures the wrapper's
     actual width, sets --pin-scale, and sets the wrapper's height to match —
     recomputed on resize so it stays correct across breakpoints. */
  const PIN_W = 345, PIN_H = 714;
  const scalePinEmbed = (el) => {
    const wrap = el.parentElement;
    const scale = Math.min(1, wrap.clientWidth / PIN_W);
    el.style.transform = `scale(${scale})`;
    wrap.style.height = `${PIN_H * scale}px`;
  };
  // Layered on purpose: a plain resize listener alone races the Tailwind Play
  // CDN, which injects its stylesheet (and therefore the real grid width)
  // asynchronously — injecting a <style> tag fires no resize event, so a
  // single early measurement can lock in the wrong scale with nothing to
  // correct it. ResizeObserver catches the wrapper's box changing regardless
  // of *why*, window resize covers viewport changes, and the two delayed
  // rAF calls catch load-order cases where neither observer fires in time.
  $$("[data-scale-embed]").forEach((el) => {
    const run = () => scalePinEmbed(el);
    run();
    new ResizeObserver(run).observe(el.parentElement);
    addEventListener("resize", run);
    requestAnimationFrame(() => requestAnimationFrame(run));
  });

  /* Hero background video — pause under reduced motion (CSS can't reach
     <video> playback) and fall back to the poster frame / gradient if it
     fails to load (e.g. the media file isn't present in this checkout). */
  const heroVideo = $(".hero-bg__video");
  if (heroVideo) {
    if (reduce) heroVideo.pause();
    heroVideo.addEventListener("error", () => heroVideo.closest(".hero-bg")?.classList.add("hero-bg--fallback"));
  }

  /* Parallax backdrop */
  const para = $$("[data-parallax]");
  if (!reduce && para.length) {
    const onScroll = () => para.forEach((el) => (el.style.transform = `translateY(${scrollY * +el.dataset.parallax}px)`));
    addEventListener("scroll", onScroll, { passive: true }); onScroll();
  }

  /* Auth modal */
  const modal = $("#authModal"), seg = $(".segmented", modal), submitLabel = $("[data-submit-label]", modal);
  let opener = null;
  const setTab = (tab) => {
    seg.dataset.active = tab;
    $$("[role=tab]", seg).forEach((t) => t.setAttribute("aria-selected", String(t.dataset.tab === tab)));
    submitLabel.innerHTML = (tab === "login" ? "Log in" : "Create account") + ' <i data-lucide="arrow-right" class="h-4 w-4"></i>';
    $("input[type=password]", modal).autocomplete = tab === "login" ? "current-password" : "new-password";
    lucide.createIcons();
  };
  const openAuth = (tab, from) => {
    opener = from; setTab(tab);
    modal.classList.remove("hidden"); document.body.style.overflow = "hidden";
    $("input[type=email]", modal).focus();
  };
  const closeAuth = () => {
    modal.classList.add("hidden"); document.body.style.overflow = "";
    opener?.focus();
  };
  $$("[data-open-auth]").forEach((b) => b.addEventListener("click", () => openAuth(b.dataset.openAuth, b)));
  $$("[data-close-auth]", modal).forEach((b) => b.addEventListener("click", closeAuth));
  $$("[role=tab]", seg).forEach((t) => t.addEventListener("click", () => setTab(t.dataset.tab)));
  document.addEventListener("keydown", (e) => {
    if (modal.classList.contains("hidden")) return;
    if (e.key === "Escape") return closeAuth();
    if (e.key !== "Tab") return;
    const f = $$("button, input, [tabindex]:not([tabindex='-1'])", modal).filter((el) => !el.disabled && el.offsetParent);
    const first = f[0], last = f[f.length - 1];
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  });
  $("form", modal).addEventListener("submit", (e) => {
    e.preventDefault();
    const btn = submitLabel, txt = btn.innerHTML;
    btn.disabled = true; btn.textContent = "…";
    setTimeout(() => { btn.disabled = false; btn.innerHTML = txt; lucide.createIcons(); closeAuth(); }, 900);
  });
})();
