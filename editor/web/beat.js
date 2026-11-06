// The beat grid's pure parts: what a click on a step does, the default
// accents, and fitting patterns and accents to a new steps-per-beat.

/** The step characters in click order: rest, hit, ghost hit. */
const CYCLE = [".", "x", "-"];

/** The next character of a step after a click. */
export function nextStep(ch) {
  const i = CYCLE.indexOf(ch);
  return CYCLE[(i + 1) % CYCLE.length];
}

/** pattern with step i clicked. */
export function toggleStep(pattern, i) {
  return pattern.slice(0, i) + nextStep(pattern[i] ?? ".") + pattern.slice(i + 1);
}

/** The accent levels a click cycles through: none, medium, strong. */
const ACCENT_LEVELS = [0, 0.6, 1];

/** The next accent level after a click; a value between levels goes up to the next one. */
export function nextAccent(a) {
  const up = ACCENT_LEVELS.find((l) => l > a + 1e-6);
  return up ?? ACCENT_LEVELS[0];
}

/**
 * The accents the library uses when a rhythm has none: beats 1 and 3
 * strong (1), 2 and 4 medium (0.6), the steps between them none.
 */
export function defaultAccents(division) {
  return Array.from({ length: 4 * division }, (_, i) => (i % division ? 0 : i % (2 * division) ? 0.6 : 1));
}

/**
 * Fits a bar of steps to a new length: a step that falls on a position of
 * the new grid keeps its value, the other new steps get fill. Going from 4
 * to 2 steps per beat keeps the eighths; from 4 to 3 only the beats.
 * @template T
 * @param {T[]} steps
 * @param {number} length
 * @param {T} fill
 * @returns {T[]}
 */
export function resample(steps, length, fill) {
  const old = steps.length;
  if (old === length || old === 0) return old ? steps.slice() : Array(length).fill(fill);
  return Array.from({ length }, (_, j) => ((j * old) % length === 0 ? steps[(j * old) / length] : fill));
}

/** A pattern string fitted to length steps (see resample). */
export function resamplePattern(pattern, length) {
  return resample([...pattern], length, ".").join("");
}
