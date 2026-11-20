import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { Palette } from "lucide-react";

export type Theme = "light" | "dark" | "neon" | "green" | "red" | "sky" | "holiday" | "minimal";

const storageKey = "telesrv.admin.theme";

type ThemeContextValue = {
  theme: Theme;
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
};

const ThemeContext = createContext<ThemeContextValue | null>(null);

export function applyTheme(theme: Theme) {
  document.documentElement.setAttribute("data-theme", theme);
  document.documentElement.style.colorScheme = theme === "dark" ? "dark" : "light";
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(() => initialTheme());

  useEffect(() => {
    applyTheme(theme);
    try {
      localStorage.setItem(storageKey, theme);
    } catch {
      // Theme persistence is best-effort.
    }
  }, [theme]);

  // Follow the OS preference until the user makes an explicit choice.
  useEffect(() => {
    if (!window.matchMedia) {
      return;
    }
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = (event: MediaQueryListEvent) => {
      let stored: string | null = null;
      try {
        stored = localStorage.getItem(storageKey);
      } catch {
        stored = null;
      }
      if (!stored || !["light", "dark", "neon", "green", "red", "sky", "holiday", "minimal"].includes(stored)) {
        setThemeState(event.matches ? "dark" : "light");
      }
    };
    media.addEventListener("change", onChange);
    return () => media.removeEventListener("change", onChange);
  }, []);

  const setTheme = useCallback((next: Theme) => setThemeState(next), []);
  const toggleTheme = useCallback(() => setThemeState((current) => (current === "dark" ? "light" : "dark")), []);

  const value = useMemo<ThemeContextValue>(() => ({ theme, setTheme, toggleTheme }), [theme, setTheme, toggleTheme]);

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const value = useContext(ThemeContext);
  if (!value) {
    throw new Error("useTheme must be used inside ThemeProvider");
  }
  return value;
}

export function ThemeSwitch() {
  const { theme, setTheme } = useTheme();
  return (
    <label className="theme-picker" title="Admin theme">
      <Palette size={15} aria-hidden="true" />
      <select value={theme} aria-label="Admin theme" onChange={(event) => setTheme(event.target.value as Theme)}>
        <option value="light">Light</option><option value="dark">Dark</option>
        <option value="neon">Neon</option><option value="green">Green</option>
        <option value="red">Red</option><option value="sky">Sky</option>
        <option value="holiday">Holiday</option><option value="minimal">Minimal</option>
      </select>
    </label>
  );
}

function initialTheme(): Theme {
  try {
    const stored = localStorage.getItem(storageKey);
    if (stored && ["light", "dark", "neon", "green", "red", "sky", "holiday", "minimal"].includes(stored)) {
      return stored as Theme;
    }
  } catch {
    // Storage is optional.
  }
  try {
    if (window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches) {
      return "dark";
    }
  } catch {
    // matchMedia can be unavailable in unusual embedded contexts.
  }
  return "light";
}
