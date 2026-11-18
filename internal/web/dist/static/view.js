import { h, clear, copyText } from './dom.js';
import { b64urlDecode } from './b64url.js';
import { decrypt, KEY_BYTES } from './crypto.js';
import { decodeEnvelope, languageLabel } from './envelope.js';
import { getStatus, consume } from './api.js';
import { renderSecret } from './render.js';

const $ = (id) => document.getElementById(id);
const AUTO_HIDE_MS = 30 * 1000;
const AUTO_CLEAR_MS = 5 * 60 * 1000;

function show(state) {
  for (const el of document.querySelectorAll('[data-state]')) el.hidden = el.dataset.state !== state;
  const active = document.querySelector(`[data-state="${state}"]`);
  const heading = active && active.querySelector('h1');
  if (heading) heading.focus();
}

const m = /^\/s\/([^/]+)\/?$/.exec(location.pathname);
const id = m ? decodeURIComponent(m[1]) : '';
let key = location.hash.replace(/^#/, '');

function keyLooksValid(k) {
  try { return b64urlDecode(k).length === KEY_BYTES; } catch (e) { return false; }
}

function errorState(r) {
  $('error-message').textContent = r.code === 'rate_limited'
    ? 'Too many requests right now. Please wait a moment.'
    : 'The secret could not be checked right now.';
  show('error');
}

async function start() {
  if (!id || !keyLooksValid(key)) { show('nokey'); return; }
  show('loading');
  const r = await getStatus(id);
  if (r.ok) { show('interstitial'); return; }
  if (r.status === 410) show('viewed');
  else if (r.status === 404) show('expired');
  else errorState(r);
}

const revealBtn = $('reveal-btn');
revealBtn.addEventListener('click', async () => {
  const box = $('reveal-error');
  box.hidden = true;
  revealBtn.disabled = true;
  try {
    const r = await consume(id);
    if (!r.ok) {
      if (r.status === 410) { show('viewed'); return; }
      if (r.status === 404) { show('expired'); return; }
      clear(box);
      box.className = 'alert danger';
      box.append(h('div', {}, h('strong', {}, 'Could not reveal the secret.'), 'Nothing was revealed. Please try again.'));
      box.hidden = false;
      return;
    }
    // Strip the key from the address bar and history as soon as the secret is ours.
    history.replaceState(null, '', location.pathname + location.search);
    const k = key;
    key = '';
    let env;
    try {
      env = decodeEnvelope(await decrypt(r.ciphertext, r.iv, k));
    } catch (e) {
      show('undecryptable');
      return;
    }
    showSecret(env);
  } finally {
    revealBtn.disabled = false;
  }
});

$('retry-btn').addEventListener('click', start);

function showSecret(env) {
  let secret = env.text;
  let shown = false;
  let hideTimer = null;
  const mask = $('secret-mask');
  const rendered = $('secret-rendered');
  const toggle = $('toggle-secret');
  const toast = $('toast');
  $('lang-badge').textContent = languageLabel(env.lang);

  function setShown(on) {
    shown = on;
    clearTimeout(hideTimer);
    toggle.setAttribute('aria-pressed', String(on));
    toggle.textContent = on ? 'Hide' : 'Show';
    mask.hidden = on;
    rendered.hidden = !on;
    if (on) {
      renderSecret(document, rendered, env.lang, secret);
      hideTimer = setTimeout(() => setShown(false), AUTO_HIDE_MS);
    } else {
      clear(rendered); // keep plaintext out of the DOM while masked
    }
  }
  toggle.addEventListener('click', () => setShown(!shown));
  $('copy-secret').addEventListener('click', async () => {
    toast.textContent = (await copyText(secret)) ? 'Copied to clipboard.' : 'Copy failed. Show the secret and copy it manually.';
  });
  show('revealed');
  setTimeout(() => {
    secret = '';
    clear(rendered);
    clearTimeout(hideTimer);
    show('cleared');
  }, AUTO_CLEAR_MS);
}

start();
