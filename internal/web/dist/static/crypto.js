// AES-256-GCM via WebCrypto. Random 12-byte IV, no AAD. The key never leaves the
// browser: it is returned as base64url for the URL fragment.
import { b64urlEncode, b64urlDecode } from './b64url.js';

export const IV_BYTES = 12;
export const KEY_BYTES = 32;
export const TAG_BYTES = 16;

function subtle() {
  const c = globalThis.crypto;
  if (!c || !c.subtle) throw new Error('WebCrypto is unavailable (a secure context, HTTPS or localhost, is required)');
  return c;
}

export async function encrypt(plaintext) {
  const c = subtle();
  const rawKey = c.getRandomValues(new Uint8Array(KEY_BYTES));
  const iv = c.getRandomValues(new Uint8Array(IV_BYTES));
  const key = await c.subtle.importKey('raw', rawKey, { name: 'AES-GCM' }, false, ['encrypt']);
  const ct = new Uint8Array(await c.subtle.encrypt({ name: 'AES-GCM', iv }, key, plaintext));
  return { ciphertext: ct, iv, key: b64urlEncode(rawKey) };
}

export async function decrypt(ciphertext, iv, keyB64) {
  const c = subtle();
  const rawKey = b64urlDecode(keyB64);
  if (rawKey.length !== KEY_BYTES) throw new Error('invalid key length');
  if (iv.length !== IV_BYTES) throw new Error('invalid iv length');
  const key = await c.subtle.importKey('raw', rawKey, { name: 'AES-GCM' }, false, ['decrypt']);
  return new Uint8Array(await c.subtle.decrypt({ name: 'AES-GCM', iv }, key, ciphertext));
}
