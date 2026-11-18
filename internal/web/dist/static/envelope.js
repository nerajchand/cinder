// The encrypted payload is the UTF-8 JSON envelope {"lang": "...", "text": "..."}.
// The size limit applies to the whole envelope. The server caps the stored
// ciphertext (envelope + 16 byte GCM tag) at 64 KiB, so the envelope may be at
// most 64 KiB minus the tag.
import { TAG_BYTES } from './crypto.js';

export const MAX_CIPHERTEXT_BYTES = 65536;
export const MAX_ENVELOPE_BYTES = MAX_CIPHERTEXT_BYTES - TAG_BYTES;

export const LANGUAGES = [
  ['text', 'Plain text'], ['markdown', 'Markdown'], ['json', 'JSON'], ['yaml', 'YAML'],
  ['bash', 'Bash'], ['go', 'Go'], ['python', 'Python'], ['javascript', 'JavaScript'],
  ['typescript', 'TypeScript'], ['sql', 'SQL'], ['html', 'HTML'], ['xml', 'XML'],
  ['toml', 'TOML'], ['dockerfile', 'Dockerfile'],
];
const LANG_IDS = new Set(LANGUAGES.map((l) => l[0]));

export function languageLabel(id) {
  const l = LANGUAGES.find((x) => x[0] === id);
  return l ? l[1] : 'Plain text';
}

const enc = new TextEncoder();
const dec = new TextDecoder('utf-8', { fatal: true });

export function encodeEnvelope(lang, text) {
  return enc.encode(JSON.stringify({ lang: LANG_IDS.has(lang) ? lang : 'text', text: String(text) }));
}

export function envelopeSize(lang, text) {
  return encodeEnvelope(lang, text).length;
}

export function decodeEnvelope(bytes) {
  const obj = JSON.parse(dec.decode(bytes));
  if (!obj || typeof obj !== 'object' || typeof obj.text !== 'string') throw new Error('invalid envelope');
  return { lang: LANG_IDS.has(obj.lang) ? obj.lang : 'text', text: obj.text };
}
