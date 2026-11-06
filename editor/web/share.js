// Share links: the voice document deflated and base64url-encoded into the
// page address (#v=...), so a link opens the same document without a server.

const PREFIX = "#v=";

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

/** The address fragment that carries text. */
export async function encodeShare(text) {
  return PREFIX + toBase64url(await pipe(new TextEncoder().encode(text), new CompressionStream("deflate-raw")));
}

/** The text a fragment carries, or null if it is not a share fragment (or is damaged). */
export async function decodeShare(hash) {
  if (!hash.startsWith(PREFIX)) return null;
  try {
    return new TextDecoder().decode(await pipe(fromBase64url(hash.slice(PREFIX.length)), new DecompressionStream("deflate-raw")));
  } catch {
    return null;
  }
}
