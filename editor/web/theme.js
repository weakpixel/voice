// The Theme button: the colour scheme follows the system until the user picks
// light or dark; the choice is kept in local storage and applied before the
// first paint by the small script in index.html.

const KEY = "voice-editor/theme";
const ORDER = ["system", "light", "dark"];

/** @returns {string} "system", "light" or "dark" */
export function savedTheme() {
  try {
    const t = localStorage.getItem(KEY);
    return t === "light" || t === "dark" ? t : "system";
  } catch {
    return "system";
  }
}

/** The theme after theme in the button's cycle. @param {string} theme */
export function nextTheme(theme) {
  return ORDER[(ORDER.indexOf(theme) + 1) % ORDER.length];
}

/** @param {string} theme */
function apply(theme) {
  if (theme === "system") delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = theme;
  try {
    if (theme === "system") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, theme);
  } catch {
    // storage blocked: the choice lasts until the page closes
  }
}

/**
 * Wires the button. onChange runs after the scheme changed, for pictures
 * drawn on a canvas that read the colours when they draw.
 * @param {HTMLElement} button
 * @param {() => void} onChange
 */
export function initTheme(button, onChange) {
  let theme = savedTheme();
  const label = () => {
    button.textContent = `Theme: ${theme[0].toUpperCase()}${theme.slice(1)}`;
  };
  label();
  button.addEventListener("click", () => {
    theme = nextTheme(theme);
    apply(theme);
    label();
    onChange();
  });
  matchMedia("(prefers-color-scheme: dark)").addEventListener("change", onChange);
}
