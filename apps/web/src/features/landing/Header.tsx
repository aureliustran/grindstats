import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Activity, ArrowRight, Menu, Moon, Sun, X } from "lucide-react";
import { useScrollSpy } from "./effects";

type ModalTab = "login" | "signup";

interface HeaderProps {
  onOpenModal: (tab: ModalTab, opener: HTMLButtonElement) => void;
}

const THEME_KEY = "gs-theme";

function getInitialIsDark(): boolean {
  if (typeof window === "undefined") return true;
  const saved = localStorage.getItem(THEME_KEY);
  if (saved) return saved === "dark";
  const explicit = document.documentElement.getAttribute("data-theme");
  if (explicit) return explicit === "dark";
  return document.documentElement.classList.contains("dark") ||
    window.matchMedia("(prefers-color-scheme: dark)").matches;
}

export function Header({ onOpenModal }: HeaderProps) {
  const { t } = useTranslation();
  const [isDark, setIsDark] = useState(getInitialIsDark);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuButtonRef = useRef<HTMLButtonElement>(null);
  /** Tracks whether the visitor has explicitly chosen a theme this session
   * (vs. the initial value computed from a stored choice or the OS
   * preference). Only an explicit choice gets written to data-theme /
   * localStorage — otherwise every page load would freeze the OS-preference
   * default in place on first render and the site would stop following
   * later OS theme changes, even for a visitor who never touched the toggle. */
  const hasExplicitChoice = useRef(localStorage.getItem(THEME_KEY) !== null);
  const activeSection = useScrollSpy(["features", "how", "numbers", "pricing"]);

  useEffect(() => {
    document.documentElement.classList.toggle("dark", isDark);
    if (!hasExplicitChoice.current) return;
    document.documentElement.setAttribute("data-theme", isDark ? "dark" : "light");
    localStorage.setItem(THEME_KEY, isDark ? "dark" : "light");
  }, [isDark]);

  useEffect(() => {
    if (!menuOpen) return;
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setMenuOpen(false);
        menuButtonRef.current?.focus();
      }
    };
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [menuOpen]);

  const toggleTheme = () => {
    hasExplicitChoice.current = true;
    setIsDark((prev) => !prev);
  };
  const closeMenu = () => setMenuOpen(false);

  const handleLogin = (e: React.MouseEvent<HTMLButtonElement>) => {
    closeMenu();
    onOpenModal("login", e.currentTarget);
  };

  const handleSignup = (e: React.MouseEvent<HTMLButtonElement>) => {
    closeMenu();
    onOpenModal("signup", e.currentTarget);
  };

  const navItems = [
    { id: "features", href: "#features", label: t("landing.nav.features") },
    { id: "how", href: "#how", label: t("landing.nav.how_it_works") },
    { id: "numbers", href: "#numbers", label: t("landing.nav.numbers") },
    { id: "pricing", href: "#pricing", label: t("landing.nav.pricing") },
  ];

  return (
    <header className="fixed inset-x-0 top-0 z-50 px-4 pt-4">
      <nav
        className="glass mx-auto flex max-w-6xl items-center justify-between rounded-2xl px-5 py-3"
        aria-label="Primary"
      >
        <a
          href="#"
          className="flex items-center gap-2 font-display text-2xl font-black tracking-tight text-ink-0 no-underline"
        >
          <span className="grid h-8 w-8 place-items-center rounded-lg bg-accent text-accent-ink">
            <Activity aria-hidden="true" className="h-4 w-4" />
          </span>
          {t("common.app_name")}
        </a>

        <ul className="hidden items-center gap-1 md:flex list-none p-0 m-0">
          {navItems.map((item) => (
            <li key={item.id}>
              <a
                href={item.href}
                className={`nav-link ${activeSection === item.id ? "is-active" : ""}`}
              >
                {item.label}
              </a>
            </li>
          ))}
        </ul>

        <div className="hidden items-center gap-2 md:flex">
          <button
            type="button"
            onClick={toggleTheme}
            className="icon-btn"
            aria-label={t("landing.nav.toggle_theme")}
          >
            {isDark ? (
              <Sun aria-hidden="true" className="h-4 w-4" />
            ) : (
              <Moon aria-hidden="true" className="h-4 w-4" />
            )}
          </button>
          <button
            type="button"
            onClick={handleLogin}
            className="btn-ghost"
            data-open-auth="login"
          >
            {t("landing.nav.login")}
          </button>
          <button
            type="button"
            onClick={handleSignup}
            className="btn-primary"
            data-open-auth="signup"
          >
            {t("landing.nav.get_started")}
            <ArrowRight aria-hidden="true" className="h-4 w-4" />
          </button>
        </div>

        <button
          ref={menuButtonRef}
          type="button"
          className="icon-btn md:hidden"
          aria-expanded={menuOpen}
          aria-controls="mobileMenu"
          aria-label={menuOpen ? t("landing.nav.menu_close") : t("landing.nav.menu_open")}
          onClick={() => setMenuOpen((prev) => !prev)}
        >
          {menuOpen ? (
            <X aria-hidden="true" className="h-5 w-5" />
          ) : (
            <Menu aria-hidden="true" className="h-5 w-5" />
          )}
        </button>
      </nav>

      {menuOpen && (
        <div
          id="mobileMenu"
          className="glass mx-auto mt-2 max-w-6xl rounded-2xl p-4 md:hidden"
        >
          <ul className="flex flex-col gap-1 list-none p-0 m-0">
            {navItems.map((item) => (
              <li key={item.id}>
                <a
                  href={item.href}
                  onClick={closeMenu}
                  className={`nav-link block ${activeSection === item.id ? "is-active" : ""}`}
                >
                  {item.label}
                </a>
              </li>
            ))}
          </ul>
          <div className="mt-3 flex gap-2 border-t border-paper-3/40 pt-3">
            <button
              type="button"
              onClick={handleLogin}
              className="btn-ghost flex-1"
              data-open-auth="login"
            >
              {t("landing.nav.login")}
            </button>
            <button
              type="button"
              onClick={handleSignup}
              className="btn-primary flex-1"
              data-open-auth="signup"
            >
              {t("landing.nav.get_started")}
            </button>
          </div>
        </div>
      )}
    </header>
  );
}
