// Thin fetch wrappers. Never log or persist bodies.
import { b64urlEncode, b64urlDecode } from './b64url.js';

async function call(method, path, body) {
  const init = { method, cache: 'no-store', credentials: 'omit', referrerPolicy: 'no-referrer', headers: {} };
  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  let res;
  try {
    res = await fetch(path, init);
  } catch (e) {
    return { ok: false, status: 0, code: 'network' };
  }
  let data = null;
  try { data = await res.json(); } catch (e) { /* non-JSON body */ }
  if (res.ok) return { ok: true, status: res.status, data };
  const code = data && data.error && data.error.code ? data.error.code : 'unknown';
  return { ok: false, status: res.status, code, retryAfter: res.headers.get('Retry-After') };
}

const enc = encodeURIComponent;

export function createSecret(ciphertext, iv, ttlSeconds) {
  return call('POST', '/api/secrets', { ciphertext: b64urlEncode(ciphertext), iv: b64urlEncode(iv), ttlSeconds });
}

export function getStatus(id) {
  return call('GET', `/api/secrets/${enc(id)}/status`);
}

export async function consume(id) {
  const r = await call('POST', `/api/secrets/${enc(id)}/consume`, {});
  if (r.ok) {
    try {
      r.ciphertext = b64urlDecode(r.data.ciphertext);
      r.iv = b64urlDecode(r.data.iv);
    } catch (e) {
      return { ok: false, status: r.status, code: 'bad_response' };
    }
  }
  return r;
}
