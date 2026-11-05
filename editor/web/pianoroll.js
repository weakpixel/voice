// The piano roll and the waveform under it: the line's notes (coloured by
// instrument, percussion in a band at the bottom), the reveal ticks, the
// offline render's waveform and a playhead, all on one time axis.

/**
 * @typedef {{on: number, off: number, instrument: string, inst: number, note: number, velocity: number}} Note
 * @typedef {{notes: Note[], reveals: {at: number, offset: number}[], duration: number, textLen: number}} Timeline
 */

/** Instruments 0 to 4 are melodic; 5 and up are percussion sounds. */
export const FIRST_PERCUSSION = 5;
const MELODIC = ["vowel", "sonorant", "fricative", "plosive", "motif"];
const PERC_BAND = 12; // px of the percussion band at the bottom of the roll
const MIN_SPAN = 12; // semitones the roll shows at least

/**
 * The pitch range and time axis of a timeline: lo and hi are the lowest
 * and highest melodic note with a semitone of room (at least MIN_SPAN
 * apart), end is the axis end in seconds (the longer of the line and the
 * render).
 * @param {Timeline | null} tl
 * @param {number} renderDuration
 */
export function layout(tl, renderDuration = 0) {
  const melodic = (tl?.notes || []).filter((n) => n.inst < FIRST_PERCUSSION).map((n) => n.note);
  let lo = melodic.length ? Math.min(...melodic) - 1 : 54;
  let hi = melodic.length ? Math.max(...melodic) + 1 : 66;
  if (hi - lo < MIN_SPAN) {
    const mid = (lo + hi) / 2;
    lo = mid - MIN_SPAN / 2;
    hi = mid + MIN_SPAN / 2;
  }
  const end = Math.max(tl?.duration || 0, renderDuration, 0.5);
  return { lo, hi, end, percussion: (tl?.notes || []).some((n) => n.inst >= FIRST_PERCUSSION) };
}

export class PianoRoll {
  /**
   * @param {HTMLCanvasElement} roll
   * @param {HTMLCanvasElement} wave
   */
  constructor(roll, wave) {
    this.roll = roll;
    this.wave = wave;
    /** @type {Timeline | null} */
    this.tl = null;
    /** @type {Float32Array | null} */
    this.peaks = null;
    this.renderDuration = 0;
    /** @type {number | null} */
    this.time = null;
    new ResizeObserver(() => this.draw()).observe(roll);
  }

  /** @param {Timeline | null} tl */
  setTimeline(tl) {
    this.tl = tl;
    this.roll.dataset.notes = String(tl?.notes.length ?? 0);
    this.draw();
  }

  /** @param {Float32Array | null} peaks min and max per column @param {number} duration seconds */
  setWave(peaks, duration) {
    this.peaks = peaks;
    this.renderDuration = duration;
    this.wave.dataset.columns = String(peaks ? peaks.length / 2 : 0);
    this.draw();
  }

  /** The playhead in seconds, null to hide it. */
  setTime(t) {
    this.time = t;
    this.draw();
  }

  /** Columns the waveform should have: its width in device pixels. */
  columns() {
    return Math.max(1, Math.round(this.wave.clientWidth * devicePixelRatio));
  }

  draw() {
    const l = layout(this.tl, this.renderDuration);
    const css = getComputedStyle(document.documentElement);
    const color = (name) => css.getPropertyValue(name).trim() || "#888";
    this.drawRoll(l, color);
    this.drawWave(l, color);
  }

  /** Sizes a canvas to its box in device pixels; returns its 2D context. */
  static context(canvas) {
    const w = Math.max(1, Math.round(canvas.clientWidth * devicePixelRatio));
    const h = Math.max(1, Math.round(canvas.clientHeight * devicePixelRatio));
    if (canvas.width !== w || canvas.height !== h) {
      canvas.width = w;
      canvas.height = h;
    }
    const g = /** @type {CanvasRenderingContext2D} */ (canvas.getContext("2d"));
    g.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0);
    g.clearRect(0, 0, canvas.clientWidth, canvas.clientHeight);
    return g;
  }

  drawRoll(l, color) {
    const c = this.roll;
    const g = PianoRoll.context(c);
    const w = c.clientWidth;
    const band = l.percussion ? PERC_BAND : 0;
    const h = c.clientHeight - band;
    const x = (t) => (t / l.end) * w;
    const y = (n) => h - ((n - l.lo) / (l.hi - l.lo)) * h;
    const row = h / (l.hi - l.lo);

    // A faint line at every C, so the octaves are easy to see.
    g.fillStyle = color("--grid");
    for (let n = Math.ceil(l.lo / 12) * 12; n <= l.hi; n += 12) g.fillRect(0, Math.round(y(n)), w, 1);
    if (band) g.fillRect(0, h, w, 1);

    for (const r of this.tl?.reveals || []) {
      g.fillStyle = color("--grid");
      g.fillRect(Math.round(x(r.at)), 0, 1, 4);
    }
    for (const n of this.tl?.notes || []) {
      const x0 = x(n.on);
      const x1 = Math.max(x(n.off), x0 + 2);
      g.globalAlpha = 0.45 + 0.55 * Math.min(1, n.velocity);
      if (n.inst >= FIRST_PERCUSSION) {
        g.fillStyle = color("--inst-percussion");
        g.fillRect(x0, h + 3, Math.max(2, Math.min(x1 - x0, 4)), band - 5);
      } else {
        g.fillStyle = color(`--inst-${MELODIC[n.inst]}`);
        g.fillRect(x0, y(n.note) - row / 2, x1 - x0, Math.max(2, row - 1));
      }
    }
    g.globalAlpha = 1;
    this.playhead(g, x, c.clientHeight, color);
  }

  drawWave(l, color) {
    const c = this.wave;
    const g = PianoRoll.context(c);
    const w = c.clientWidth;
    const h = c.clientHeight;
    const mid = h / 2;
    g.fillStyle = color("--grid");
    g.fillRect(0, Math.round(mid), w, 1);
    const p = this.peaks;
    if (p && p.length) {
      const cols = p.length / 2;
      const span = (this.renderDuration / l.end) * w; // the render covers this much of the axis
      g.fillStyle = color("--wave");
      for (let i = 0; i < cols; i++) {
        const x0 = (i / cols) * span;
        const lo = Math.max(-1, p[2 * i]);
        const hi = Math.min(1, p[2 * i + 1]);
        g.fillRect(x0, mid - hi * mid, Math.max(1, span / cols), Math.max(1, (hi - lo) * mid));
      }
    }
    this.playhead(g, (t) => (t / l.end) * w, h, color);
  }

  playhead(g, x, h, color) {
    if (this.time === null) return;
    g.fillStyle = color("--playhead");
    g.fillRect(Math.round(x(this.time)), 0, 2, h);
  }
}
