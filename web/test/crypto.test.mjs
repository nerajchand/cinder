import test from 'node:test';
import assert from 'node:assert/strict';
import { encrypt, decrypt, IV_BYTES, TAG_BYTES } from '../../internal/web/dist/static/crypto.js';
import { b64urlEncode, b64urlDecode } from '../../internal/web/dist/static/b64url.js';
import { encodeEnvelope, decodeEnvelope, envelopeSize, MAX_ENVELOPE_BYTES, MAX_CIPHERTEXT_BYTES } from '../../internal/web/dist/static/envelope.js';

const td = new TextDecoder();

test('round trip through envelope', async () => {
  const env = encodeEnvelope('go', 'héllo ✓ "quoted" \n newline');
  const { ciphertext, iv, key } = await encrypt(env);
  assert.equal(iv.length, IV_BYTES);
  assert.equal(ciphertext.length, env.length + TAG_BYTES);
  assert.match(key, /^[A-Za-z0-9_-]{43}$/); // 32 bytes, base64url, no padding
  const out = decodeEnvelope(await decrypt(ciphertext, iv, key));
  assert.deepEqual(out, { lang: 'go', text: 'héllo ✓ "quoted" \n newline' });
});

test('plaintext never appears in ciphertext', async () => {
  const { ciphertext } = await encrypt(encodeEnvelope('text', 'SUPERSECRETVALUE'));
  assert.ok(!td.decode(ciphertext).includes('SUPERSECRETVALUE'));
});

test('wrong key fails', async () => {
  const a = await encrypt(encodeEnvelope('text', 'x'));
  const b = await encrypt(encodeEnvelope('text', 'x'));
  await assert.rejects(decrypt(a.ciphertext, a.iv, b.key));
});

test('tampered ciphertext, tag and iv fail', async () => {
  const { ciphertext, iv, key } = await encrypt(encodeEnvelope('text', 'tamper me'));
  for (const idx of [0, ciphertext.length - 1]) {
    const bad = ciphertext.slice(); bad[idx] ^= 1;
    await assert.rejects(decrypt(bad, iv, key));
  }
  const badIv = iv.slice(); badIv[0] ^= 1;
  await assert.rejects(decrypt(ciphertext, badIv, key));
});

test('invalid key / iv input is rejected', async () => {
  const { ciphertext, iv } = await encrypt(encodeEnvelope('text', 'x'));
  await assert.rejects(decrypt(ciphertext, iv, 'short'));
  await assert.rejects(decrypt(ciphertext, iv, '!!!'));
  await assert.rejects(decrypt(ciphertext, iv.slice(0, 8), 'A'.repeat(43)));
});

test('IVs and keys are unique across encryptions', async () => {
  const ivs = new Set(); const keys = new Set();
  for (let i = 0; i < 200; i++) {
    const r = await encrypt(encodeEnvelope('text', 'same'));
    ivs.add(b64urlEncode(r.iv)); keys.add(r.key);
  }
  assert.equal(ivs.size, 200);
  assert.equal(keys.size, 200);
});

test('base64url is unpadded and url-safe, round trips all lengths', () => {
  for (let n = 0; n < 40; n++) {
    const b = new Uint8Array(n).map((_, i) => (i * 37 + 250) & 255);
    const s = b64urlEncode(b);
    assert.match(s, /^[A-Za-z0-9_-]*$/);
    assert.deepEqual(b64urlDecode(s), b);
  }
  assert.throws(() => b64urlDecode('abc='));
  assert.throws(() => b64urlDecode('a+b/'));
  assert.throws(() => b64urlDecode('A'));
});

test('envelope size limit covers the whole JSON envelope and fits the server cap', () => {
  assert.equal(MAX_ENVELOPE_BYTES + 16, MAX_CIPHERTEXT_BYTES);
  assert.equal(envelopeSize('text', ''), JSON.stringify({ lang: 'text', text: '' }).length);
  // multi-byte and escaped characters count by their JSON-encoded UTF-8 size
  assert.equal(envelopeSize('text', '✓'), JSON.stringify({ lang: 'text', text: '✓' }).length + 2);
  assert.equal(envelopeSize('text', '"'), JSON.stringify({ lang: 'text', text: '"' }).length);
  const overhead = envelopeSize('markdown', '');
  assert.equal(envelopeSize('markdown', 'a'.repeat(MAX_ENVELOPE_BYTES - overhead)), MAX_ENVELOPE_BYTES);
  assert.ok(envelopeSize('markdown', 'a'.repeat(MAX_ENVELOPE_BYTES - overhead + 1)) > MAX_ENVELOPE_BYTES);
});

test('envelope: unknown language falls back to text; garbage rejected', () => {
  assert.equal(decodeEnvelope(new TextEncoder().encode('{"lang":"klingon","text":"x"}')).lang, 'text');
  assert.throws(() => decodeEnvelope(new TextEncoder().encode('not json')));
  assert.throws(() => decodeEnvelope(new TextEncoder().encode('{"lang":"go"}')));
  assert.equal(decodeEnvelope(encodeEnvelope('nope', 'x')).lang, 'text');
});
