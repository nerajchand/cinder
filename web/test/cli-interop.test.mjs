import test from 'node:test';
import assert from 'node:assert/strict';
import { encryptEnvelope, decryptEnvelope } from '../../scripts/cinder-cli.mjs';
import { encrypt, decrypt } from '../../internal/web/dist/static/crypto.js';
import { b64urlEncode, b64urlDecode } from '../../internal/web/dist/static/b64url.js';
import { encodeEnvelope, decodeEnvelope } from '../../internal/web/dist/static/envelope.js';

const text = 'héllo ✓ "quoted"\nline2 \u{1F525}';

test('CLI-encrypted secret decrypts with the browser modules', async () => {
  const e = encryptEnvelope(text, 'bash');
  assert.match(e.key, /^[A-Za-z0-9_-]{43}$/);
  assert.equal(b64urlDecode(e.iv).length, 12);
  const plain = await decrypt(b64urlDecode(e.ciphertext), b64urlDecode(e.iv), e.key);
  assert.deepEqual(decodeEnvelope(plain), { lang: 'bash', text });
});

test('browser-encrypted secret decrypts with the CLI', async () => {
  const r = await encrypt(encodeEnvelope('go', text));
  const out = decryptEnvelope(b64urlEncode(r.ciphertext), b64urlEncode(r.iv), r.key);
  assert.deepEqual(out, { lang: 'go', text });
});

test('CLI rejects tampered ciphertext and wrong key', () => {
  const e = encryptEnvelope('x');
  const bad = Buffer.from(e.ciphertext, 'base64url');
  bad[0] ^= 1;
  assert.throws(() => decryptEnvelope(bad.toString('base64url'), e.iv, e.key));
  assert.throws(() => decryptEnvelope(e.ciphertext, e.iv, encryptEnvelope('y').key));
});
