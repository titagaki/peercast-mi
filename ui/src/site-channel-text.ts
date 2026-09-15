import type { SiteChannel } from "./site-api";

export function channelExplanation(channel: SiteChannel) {
  // Match peca-live's grouping without deleting substrings in genre names
  // (e.g. "Endgame") or interpreting broadcast text as HTML.
  // YP4G: prefix [alphanumeric namespace:] [?] [@...] genre.
  // Strip only recognizable control syntax: an already cleaned word such as
  // "sports" must not lose its first two letters. See docs/yp/genre.md in
  // peca-docs and the verification record in docs/reviews/.
  const genre = channel.genre
    .trim()
    .replace(
      /^(?:yp|sp|tp|pp)(?:[a-zA-Z0-9]+:\??@*|\?@*|@+|(?=$|[^a-zA-Z0-9]))/,
      "",
    )
    .replace(/\bgame\b/gi, "")
    .replace(/^[\s-]+|[\s-]+$/g, "");
  const description = channel.description
    .replace(/(?:\s*-\s*)?<(?:Open|Free|2M Over|Over)>/g, "")
    .trim();
  return [genre, [description, channel.comment].filter(Boolean).join(" ")]
    .filter(Boolean)
    .join(" - ");
}
