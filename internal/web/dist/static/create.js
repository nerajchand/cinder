import { h, clear, copyText } from './dom.js';
import { encodeEnvelope, MAX_ENVELOPE_BYTES } from './envelope.js';
import { encrypt } from './crypto.js';
import { createSecret } from './api.js';
import { renderSecret } from './render.js';
import { encodeText, drawQR } from './vendor/qrcode.js';

const $ = (id) => document.getElementById(id);
const form = $('create-form');
const ta = $('secret-input');
const langSel = $('lang');
const ttlSel = $('ttl');
const counter = $('counter');
const submit = $('submit-btn');
const errBox = $('form-error');
const nf = new Intl.NumberFormat('en-US');

function showError(msg) {
  clear(errBox);
  errBox.className = 'alert danger';
  errBox.append(h('div', {}, h('strong', {}, 'Could not create the link.'), msg));
  errBox.hidden = false;
}

function updateCounter() {
  const n = encodeEnvelope(langSel.value, ta.value).length;
  const over = n > MAX_ENVELOPE_BYTES;
  counter.textContent = `${nf.format(n)} / ${nf.format(MAX_ENVELOPE_BYTES)} bytes (64 KiB)`;
  counter.classList.toggle('over', over);
  return { n, over };
}

/* Write / Preview tabs */
const tabs = [...document.querySelectorAll('[role=tab]')];
const preview = $('panel-preview');
function selectTab(tab) {
  tabs.forEach((o) => {
    const on = o === tab;
    o.setAttribute('aria-selected', String(on));
    o.tabIndex = on ? 0 : -1;
    $(o.getAttribute('aria-controls')).hidden = !on;
  });
  if (tab.id === 'tab-preview') {
    if (ta.value) renderSecret(document, preview, langSel.value, ta.value);
    else { clear(preview); preview.append(h('p', { class: 'preview-empty' }, 'Nothing to preview.')); }
  } else {
    clear(preview); // do not keep a rendered copy around
  }
}
tabs.forEach((tab) => {
  tab.addEventListener('click', () => selectTab(tab));
  tab.addEventListener('keydown', (e) => {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
    const next = tabs[(tabs.indexOf(tab) + (e.key === 'ArrowRight' ? 1 : tabs.length - 1)) % tabs.length];
    next.focus();
    selectTab(next);
  });
});

ta.addEventListener('input', updateCounter);
langSel.addEventListener('change', updateCounter);

function friendly(r) {
  switch (r.code) {
    case 'payload_too_large': return 'The secret is too large for the server. Shorten it and try again.';
    case 'rate_limited': return `Too many requests. Please wait${r.retryAfter ? ` ${r.retryAfter} seconds` : ' a moment'} and try again.`;
    case 'unavailable': return 'The service is temporarily unavailable. Please try again shortly.';
    case 'network': return 'Network error. Check your connection and try again.';
    default: return 'The server rejected the request. Please try again.';
  }
}

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  errBox.hidden = true;
  if (!ta.value) { showError('Enter a secret first.'); ta.focus(); return; }
  const { over } = updateCounter();
  if (over) { showError('The secret is over the 64 KiB limit. Shorten it and try again.'); return; }
  submit.disabled = true;
  try {
    const enc = await encrypt(encodeEnvelope(langSel.value, ta.value));
    const r = await createSecret(enc.ciphertext, enc.iv, Number(ttlSel.value));
    if (!r.ok) { showError(friendly(r)); return; }
    ta.value = '';
    updateCounter();
    showCreated(`${location.origin}/s/${r.data.id}#${enc.key}`, r.data.expiresAt);
  } catch (err) {
    showError(err && err.message ? err.message : 'Encryption failed.');
  } finally {
    submit.disabled = false;
  }
});

/* Link page */
function showCreated(link, expiresAt) {
  $('create-card').hidden = true;
  $('created-card').hidden = false;
  const input = $('share-link');
  input.value = link;
  const t = $('expires-at');
  const d = new Date(expiresAt);
  t.setAttribute('datetime', expiresAt);
  t.textContent = Number.isNaN(d.getTime()) ? expiresAt : d.toLocaleString([], { dateStyle: 'full', timeStyle: 'short' });
  $('h-created').focus();
  const toast = $('toast');
  $('copy-link').addEventListener('click', async () => {
    toast.textContent = (await copyText(link, input)) ? 'Link copied to clipboard.' : 'Copy failed. Select the link and copy it manually.';
  });
  input.addEventListener('focus', () => input.select());
  const qb = $('qr-toggle');
  const panel = $('qr-panel');
  let drawn = false;
  qb.addEventListener('click', () => {
    const open = qb.getAttribute('aria-expanded') === 'true';
    qb.setAttribute('aria-expanded', String(!open));
    qb.textContent = open ? 'Show QR code' : 'Hide QR code';
    panel.hidden = open;
    if (!open && !drawn) {
      try { drawQR($('qr-canvas'), encodeText(link), 4, 4); drawn = true; } catch (err) { panel.hidden = true; toast.textContent = 'Could not generate a QR code for this link.'; }
    }
  });
}

$('create-card').hidden = false;
updateCounter();
