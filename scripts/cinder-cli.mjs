#!/usr/bin/env node
// Cinder command line client. Zero dependencies; needs Node 18+.
//
// Encrypts locally (AES-256-GCM, random 12-byte IV, no AAD, envelope {"lang","text"})
// exactly like the browser, so links work in both directions. The server only ever
// sees ciphertext; the key stays in the link's #fragment.
//
//   echo -n 'hunter2' | scripts/cinder-cli.mjs create --url http://localhost:8080
//   scripts/cinder-cli.mjs fetch 'http://localhost:8080/s/<id>#<key>'
import { randomBytes, createCipheriv, createDecipheriv } from 'node:crypto';
import { readFileSync } from 'node:fs';

const KEY_BYTES = 32, IV_BYTES = 12, TAG_BYTES = 16;
const TTLS = [300, 3600, 86400, 604800];
const LANGS = ['text', 'markdown', 'json', 'yaml', 'bash', 'go', 'python', 'javascript',
  'typescript', 'sql', 'html', 'xml', 'toml', 'dockerfile'];

const b64u = (buf) => Buffer.from(buf).toString('base64url');
const unb64u = (s) => {
  if (typeof s !== 'string' || !/^[A-Za-z0-9_-]*$/.test(s) || s.length % 4 === 1) throw new Error('invalid base64url');
  return Buffer.from(s, 'base64url');
};

export function encryptEnvelope(text, lang = 'text') {
  const key = randomBytes(KEY_BYTES), iv = randomBytes(IV_BYTES);
  const envelope = Buffer.from(JSON.stringify({ lang: LANGS.includes(lang) ? lang : 'text', text }), 'utf8');
  const c = createCipheriv('aes-256-gcm', key, iv);
  // WebCrypto layout: ciphertext || 16-byte tag.
  const ct = Buffer.concat([c.update(envelope), c.final(), c.getAuthTag()]);
  return { ciphertext: b64u(ct), iv: b64u(iv), key: b64u(key) };
}

export function decryptEnvelope(ciphertextB64, ivB64, keyB64) {
  const key = unb64u(keyB64), iv = unb64u(ivB64), ct = unb64u(ciphertextB64);
  if (key.length !== KEY_BYTES) throw new Error('invalid key length');
  if (iv.length !== IV_BYTES) throw new Error('invalid iv length');
  if (ct.length < TAG_BYTES) throw new Error('ciphertext too short');
  const d = createDecipheriv('aes-256-gcm', key, iv);
  d.setAuthTag(ct.subarray(ct.length - TAG_BYTES));
  const plain = Buffer.concat([d.update(ct.subarray(0, ct.length - TAG_BYTES)), d.final()]);
  const obj = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(plain));
  if (!obj || typeof obj.text !== 'string') throw new Error('invalid envelope');
  return { lang: LANGS.includes(obj.lang) ? obj.lang : 'text', text: obj.text };
}

async function call(url, init) {
  let res;
  try { res = await fetch(url, init); } catch (e) { throw new Error(`cannot reach ${url}: ${e.cause?.code || e.message}`); }
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(`${res.status} ${body.error?.code || res.statusText}: ${body.error?.message || ''}`.trim());
  return body;
}

const usage = `Usage:
  cinder-cli create [--url URL] [--ttl 300|3600|86400|604800] [--lang LANG] [--file PATH | TEXT]
      Reads the secret from --file, TEXT, or stdin. Prints the share link.
      URL defaults to $CINDER_URL or http://localhost:8080.
  cinder-cli fetch [--json] LINK
      Consumes (burns) the secret and prints the plaintext. Only run once!
`;

function parseArgs(argv) {
  const o = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--json') o.json = true;
    else if (['--url', '--ttl', '--lang', '--file'].includes(a)) {
      if (i + 1 >= argv.length) throw new Error(`${a} needs a value`);
      o[a.slice(2)] = argv[++i];
    } else if (a === '-h' || a === '--help') o.help = true;
    else if (a.startsWith('--')) throw new Error(`unknown option ${a}`);
    else o._.push(a);
  }
  return o;
}

async function create(o) {
  const base = new URL(o.url || process.env.CINDER_URL || 'http://localhost:8080');
  const ttl = Number(o.ttl || 86400);
  if (!TTLS.includes(ttl)) throw new Error(`--ttl must be one of ${TTLS.join(', ')}`);
  let text;
  if (o.file) text = readFileSync(o.file, 'utf8');
  else if (o._.length) text = o._.join(' ');
  else text = readFileSync(0, 'utf8');
  if (!text) throw new Error('empty secret');
  const enc = encryptEnvelope(text, o.lang || 'text');
  const r = await call(new URL('/api/secrets', base), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ciphertext: enc.ciphertext, iv: enc.iv, ttlSeconds: ttl }),
  });
  console.log(`${base.origin}/s/${r.id}#${enc.key}`);
  console.error(`expires ${r.expiresAt}`);
}

async function fetchSecret(o) {
  if (o._.length !== 1) throw new Error('fetch needs exactly one LINK');
  const link = new URL(o._[0]);
  const m = /^\/s\/([^/]+)\/?$/.exec(link.pathname);
  const key = link.hash.replace(/^#/, '');
  if (!m || !key) throw new Error('LINK must look like https://host/s/<id>#<key>');
  const r = await call(new URL(`/api/secrets/${m[1]}/consume`, link), {
    method: 'POST',
    // The server requires Origin to match the Host it sees.
    headers: { 'Content-Type': 'application/json', Origin: link.origin },
    body: '{}',
  });
  let out;
  try { out = decryptEnvelope(r.ciphertext, r.iv, key); }
  catch { throw new Error('secret was burned but could not be decrypted (wrong key?)'); }
  if (o.json) console.log(JSON.stringify(out));
  else process.stdout.write(out.text);
}

if (import.meta.url === `file://${process.argv[1]}` || process.argv[1]?.endsWith('cinder-cli.mjs')) {
  try {
    const o = parseArgs(process.argv.slice(3));
    const cmd = process.argv[2];
    if (!cmd || o.help || cmd === '-h' || cmd === '--help') { console.error(usage); process.exit(cmd ? 0 : 2); }
    if (cmd === 'create') await create(o);
    else if (cmd === 'fetch') await fetchSecret(o);
    else throw new Error(`unknown command ${cmd}`);
  } catch (e) {
    console.error(`cinder-cli: ${e.message}`);
    process.exit(1);
  }
}
