// Quick tooltips. The browser's own tooltip of a title attribute waits about
// a second and cannot be told otherwise, which is long when you move along
// the form reading what each field does. This one shows the same title
// after a short delay. It takes the title over on first hover (moving it to
// data-tip, so the browser's own does not show as well); the options of an
// open dropdown keep the browser's, since a page cannot draw over that list.

/** Shows the title of the hovered element after delayMs. */
export function installTooltips(delayMs = 150) {
  const tip = document.createElement("div");
  tip.className = "tooltip";
  tip.setAttribute("role", "tooltip");
  tip.hidden = true;
  document.body.append(tip);

  let owner = null;
  let timer = 0;
  let x = 0;
  let y = 0;

  const hide = () => {
    clearTimeout(timer);
    owner = null;
    tip.hidden = true;
  };

  const place = () => {
    const pad = 8;
    const w = tip.offsetWidth;
    const h = tip.offsetHeight;
    let left = Math.min(x + 12, window.innerWidth - w - pad);
    let top = y + 18;
    if (top + h > window.innerHeight - pad) top = y - h - 10;
    tip.style.left = `${Math.max(pad, left)}px`;
    tip.style.top = `${Math.max(pad, top)}px`;
  };

  document.addEventListener("pointerover", (e) => {
    if (e.pointerType === "touch" || !(e.target instanceof Element)) return;
    const el = e.target.closest("[title], [data-tip]");
    if (!el || el.tagName === "OPTION") return hide();
    if (el.hasAttribute("title")) {
      el.dataset.tip = el.getAttribute("title");
      el.removeAttribute("title");
    }
    if (el === owner) return;
    hide();
    if (!el.dataset.tip) return;
    owner = el;
    timer = setTimeout(() => {
      if (owner !== el || !el.isConnected) return;
      tip.textContent = el.dataset.tip;
      tip.hidden = false;
      place();
    }, delayMs);
  });
  document.addEventListener("pointermove", (e) => {
    x = e.clientX;
    y = e.clientY;
    if (!tip.hidden) place();
  });
  document.addEventListener("pointerout", (e) => {
    if (owner && !(e.relatedTarget instanceof Node && owner.contains(e.relatedTarget))) hide();
  });
  for (const ev of ["pointerdown", "keydown", "scroll", "blur"]) window.addEventListener(ev, hide, true);
}
