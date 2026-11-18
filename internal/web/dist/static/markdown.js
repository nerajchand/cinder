// Small sanitising markdown renderer. Two stages:
//   parseMarkdown(src) -> plain-object AST (all URL sanitising happens here)
//   renderMarkdown(src, doc) -> DocumentFragment built with createElement/createTextNode
// There is no innerHTML and no raw HTML support: "<script>" is just text.
// Images are never rendered; their source is shown as text.
import { highlightInto } from './highlight.js';

const MAX_DEPTH = 8;
const INLINE_WINDOW = 1000; // bounds closing-delimiter search so hostile input stays linear-ish
const SAFE_PROTOCOLS = new Set(['http:', 'https:', 'mailto:']);

// Returns a normalised href or null if the URL is not allowed.
export function safeHref(raw) {
  const s = String(raw).trim();
  if (!s) return null;
  let u;
  try { u = new URL(s); } catch (e) { return null; } // relative or malformed: not linkable
  if (!SAFE_PROTOCOLS.has(u.protocol)) return null;
  return u.href;
}

/* ---------- inline ---------- */

function parseLink(s, i) {
  // s[i] === '['. Returns {label, url, end} or null.
  let depth = 0;
  let j = i;
  const limit = Math.min(s.length, i + INLINE_WINDOW);
  for (; j < limit; j++) {
    const c = s[j];
    if (c === '\\') { j++; continue; }
    if (c === '[') depth++;
    else if (c === ']') { depth--; if (depth === 0) break; }
  }
  if (j >= limit || s[j + 1] !== '(') return null;
  const label = s.slice(i + 1, j);
  let k = j + 2;
  let parens = 0;
  const urlStart = k;
  const urlLimit = Math.min(s.length, k + INLINE_WINDOW);
  while (k < urlLimit) {
    const c = s[k];
    if (c === '(') parens++;
    else if (c === ')') { if (parens === 0) break; parens--; }
    else if (c === ' ' || c === '\n') break;
    k++;
  }
  const url = s.slice(urlStart, k);
  if (s[k] === ' ') { // optional "title"
    const close = s.indexOf(')', k);
    if (close < 0 || close > k + INLINE_WINDOW) return null;
    k = close;
  }
  if (s[k] !== ')') return null;
  return { label, url, end: k + 1 };
}

export function parseInline(s, depth = 0) {
  const out = [];
  let buf = '';
  let i = 0;
  const flush = () => { if (buf) { out.push({ type: 'text', text: buf }); buf = ''; } };
  while (i < s.length) {
    const c = s[i];
    if (c === '\\' && i + 1 < s.length && /[\\`*_{}[\]()#+\-.!>~|<]/.test(s[i + 1])) {
      buf += s[i + 1]; i += 2; continue;
    }
    if (c === '`') {
      let n = 1;
      while (s[i + n] === '`') n++;
      const fence = '`'.repeat(n);
      const close = s.indexOf(fence, i + n);
      if (close > 0 && close - i < INLINE_WINDOW * 4) {
        flush();
        let t = s.slice(i + n, close);
        if (t.length > 2 && t.startsWith(' ') && t.endsWith(' ')) t = t.slice(1, -1);
        out.push({ type: 'code', text: t });
        i = close + n;
      } else { buf += fence; i += n; }
      continue;
    }
    if (c === '!' && s[i + 1] === '[') {
      const l = parseLink(s, i + 1);
      if (l) { buf += s.slice(i, l.end); i = l.end; continue; } // image: shown as literal text
    }
    if (c === '[' && depth < MAX_DEPTH) {
      const l = parseLink(s, i);
      if (l) {
        flush();
        const children = parseInline(l.label, depth + 1);
        const href = safeHref(l.url);
        if (href) out.push({ type: 'link', href, children });
        else out.push(...children, { type: 'text', text: ` (${l.url})` });
        i = l.end;
        continue;
      }
    }
    if ((c === '*' || c === '_') && depth < MAX_DEPTH) {
      const n = s[i + 1] === c ? 2 : 1;
      const m = c.repeat(n);
      const after = s[i + n];
      const before = s[i - 1];
      const intraword = c === '_' && before && /\w/.test(before);
      if (after && !/\s/.test(after) && !intraword) {
        const end = Math.min(s.length, i + n + INLINE_WINDOW);
        let j = i + n + 1;
        let found = -1;
        while (j < end) {
          j = s.indexOf(m, j);
          if (j < 0 || j >= end) break;
          const prev = s[j - 1];
          const next = s[j + n];
          if (!/\s/.test(prev) && (n === 2 || next !== c) && prev !== c && !(c === '_' && next && /\w/.test(next))) { found = j; break; }
          j++;
        }
        if (found > 0) {
          flush();
          out.push({ type: n === 2 ? 'strong' : 'em', children: parseInline(s.slice(i + n, found), depth + 1) });
          i = found + n;
          continue;
        }
      }
    }
    buf += c;
    i++;
  }
  flush();
  return out;
}

/* ---------- blocks ---------- */

const RE_FENCE = /^ {0,3}(`{3,}|~{3,})\s*([\w+#.-]*)[^`]*$/;
const RE_HEADING = /^ {0,3}(#{1,6})(?:\s+(.*))?$/;
const RE_HR = /^ {0,3}([-*_])(?: *\1){2,} *$/;
const RE_QUOTE = /^ {0,3}>/;
const RE_LIST = /^( *)([-*+]|\d{1,9}[.)])( +|$)(.*)$/;

function startsBlock(line) {
  return RE_FENCE.test(line) || RE_HEADING.test(line) || RE_HR.test(line) || RE_QUOTE.test(line) || RE_LIST.test(line);
}

export function parseBlocks(lines, depth = 0) {
  const blocks = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) { i++; continue; }

    let m = RE_FENCE.exec(line);
    if (m) {
      const fence = m[1];
      const body = [];
      i++;
      while (i < lines.length) {
        const t = lines[i].trim();
        if (t.length >= fence.length && t[0] === fence[0] && t === t[0].repeat(t.length)) { i++; break; }
        body.push(lines[i]);
        i++;
      }
      blocks.push({ type: 'code', lang: (m[2] || '').toLowerCase(), text: body.join('\n') });
      continue;
    }
    if (RE_HR.test(line)) { blocks.push({ type: 'hr' }); i++; continue; }
    m = RE_HEADING.exec(line);
    if (m) {
      blocks.push({ type: 'heading', level: m[1].length, children: parseInline((m[2] || '').replace(/\s+#+\s*$/, '').trim()) });
      i++;
      continue;
    }
    if (RE_QUOTE.test(line)) {
      const inner = [];
      while (i < lines.length && RE_QUOTE.test(lines[i])) { inner.push(lines[i].replace(/^ {0,3}> ?/, '')); i++; }
      blocks.push({ type: 'blockquote', children: depth < MAX_DEPTH ? parseBlocks(inner, depth + 1) : [{ type: 'paragraph', children: [{ type: 'text', text: inner.join('\n') }] }] });
      continue;
    }
    m = RE_LIST.exec(line);
    if (m) {
      const base = m[1].length;
      const ordered = /\d/.test(m[2]);
      const items = [];
      let cur = null;
      while (i < lines.length) {
        const l = lines[i];
        const lm = RE_LIST.exec(l);
        if (lm && lm[1].length <= base + 1 && !RE_HR.test(l)) {
          cur = { lines: [lm[4]], width: lm[1].length + lm[2].length + 1 };
          items.push(cur);
          i++;
        } else if (!l.trim()) {
          let k = i;
          while (k < lines.length && !lines[k].trim()) k++;
          if (k < lines.length && /^ {2,}\S/.test(lines[k])) { cur.lines.push(''); i++; } else break;
        } else if (/^ {2,}\S/.test(l)) {
          cur.lines.push(l.slice(Math.min(cur.width, l.length - l.trimStart().length)));
          i++;
        } else if (!startsBlock(l)) { // lazy continuation
          cur.lines.push(l);
          i++;
        } else break;
      }
      blocks.push({
        type: 'list', ordered,
        items: items.map((it) => (depth < MAX_DEPTH ? parseBlocks(it.lines, depth + 1) : [{ type: 'paragraph', children: [{ type: 'text', text: it.lines.join('\n') }] }])),
      });
      continue;
    }
    const para = [];
    while (i < lines.length && lines[i].trim() && (para.length === 0 || !startsBlock(lines[i]))) { para.push(lines[i].trim()); i++; }
    blocks.push({ type: 'paragraph', children: parseInline(para.join('\n')) });
  }
  return blocks;
}

export function parseMarkdown(src) {
  return parseBlocks(String(src).replace(/\r\n?/g, '\n').split('\n'));
}

/* ---------- render ---------- */

function textWithBreaks(doc, text, parent) {
  const parts = text.split('\n');
  parts.forEach((p, idx) => {
    if (idx > 0) parent.appendChild(doc.createElement('br'));
    if (p) parent.appendChild(doc.createTextNode(p));
  });
}

function renderInline(doc, nodes, parent) {
  for (const n of nodes) {
    switch (n.type) {
      case 'text': textWithBreaks(doc, n.text, parent); break;
      case 'code': { const e = doc.createElement('code'); e.appendChild(doc.createTextNode(n.text)); parent.appendChild(e); break; }
      case 'strong': case 'em': {
        const e = doc.createElement(n.type);
        renderInline(doc, n.children, e);
        parent.appendChild(e);
        break;
      }
      case 'link': {
        const a = doc.createElement('a');
        a.setAttribute('href', n.href);
        a.setAttribute('rel', 'noopener noreferrer nofollow');
        a.setAttribute('target', '_blank');
        renderInline(doc, n.children, a);
        parent.appendChild(a);
        break;
      }
      default: break;
    }
  }
}

function renderBlocks(doc, blocks, parent) {
  for (const b of blocks) {
    switch (b.type) {
      case 'heading': {
        const e = doc.createElement('h' + Math.min(6, b.level + 1)); // keep page h1 unique
        renderInline(doc, b.children, e);
        parent.appendChild(e);
        break;
      }
      case 'paragraph': { const e = doc.createElement('p'); renderInline(doc, b.children, e); parent.appendChild(e); break; }
      case 'hr': parent.appendChild(doc.createElement('hr')); break;
      case 'blockquote': { const e = doc.createElement('blockquote'); renderBlocks(doc, b.children, e); parent.appendChild(e); break; }
      case 'list': {
        const e = doc.createElement(b.ordered ? 'ol' : 'ul');
        for (const item of b.items) { const li = doc.createElement('li'); renderBlocks(doc, item, li); e.appendChild(li); }
        parent.appendChild(e);
        break;
      }
      case 'code': {
        const pre = doc.createElement('pre');
        const code = doc.createElement('code');
        highlightInto(doc, code, b.text, b.lang);
        pre.appendChild(code);
        parent.appendChild(pre);
        break;
      }
      default: break;
    }
  }
}

export function renderMarkdown(src, doc) {
  const frag = doc.createDocumentFragment();
  renderBlocks(doc, parseMarkdown(src), frag);
  return frag;
}
