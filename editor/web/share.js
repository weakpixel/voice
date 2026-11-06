// Share links: the voice document deflated and base64url-encoded into the
// page address (#v=...), so a link opens the same document without a server.

const PREFIX = "#v=";
const DIALOGUE_PREFIX = "#d=";

async function pipe(bytes, stream) {
  const out = new Blob([bytes]).stream().pipeThrough(stream);
  return new Uint8Array(await new Response(out).arrayBuffer());
}

function toBase64url(bytes) {
  let s = "";
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function fromBase64url(text) {
  const s = atob(text.replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(s, (c) => c.charCodeAt(0));
}

async function encode(prefix, text) {
  return prefix + toBase64url(await pipe(new TextEncoder().encode(text), new CompressionStream("deflate-raw")));
}

async function decode(prefix, hash) {
  if (!hash.startsWith(prefix)) return null;
  try {
    return new TextDecoder().decode(await pipe(fromBase64url(hash.slice(prefix.length)), new DecompressionStream("deflate-raw")));
  } catch {
    return null;
  }
}

/** The address fragment that carries text. */
export function encodeShare(text) {
  return encode(PREFIX, text);
}

/** The text a fragment carries, or null if it is not a share fragment (or is damaged). */
export function decodeShare(hash) {
  return decode(PREFIX, hash);
}

/**
 * The fragment (#d=...) that carries a dialogue and the voice files it uses.
 * @param {{dialogue: string, voices: Record<string, string>}} bundle
 */
export function encodeBundle(bundle) {
  return encode(DIALOGUE_PREFIX, JSON.stringify(bundle));
}

/**
 * The bundle a #d= fragment carries, or null if it is not one (or is damaged).
 * @returns {Promise<{dialogue: string, voices: Record<string, string>} | null>}
 */
export async function decodeBundle(hash) {
  const text = await decode(DIALOGUE_PREFIX, hash);
  if (text === null) return null;
  try {
    const b = JSON.parse(text);
    if (typeof b?.dialogue !== "string" || b.voices === null || typeof b.voices !== "object") return null;
    return { dialogue: b.dialogue, voices: Object.fromEntries(Object.entries(b.voices).filter(([, v]) => typeof v === "string")) };
  } catch {
    return null;
  }
}
