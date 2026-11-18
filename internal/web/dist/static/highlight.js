// Tiny single-pass tokenizer. Emits [cssClass, text] pairs; the renderer wraps
// classed tokens in <span class="tk-..."> using textContent only.
// Classes: tk-k keyword, tk-s string, tk-c comment, tk-n number, tk-v variable,
// tk-t tag/section, tk-a attribute/key.
// Bounded: input longer than MAX_HIGHLIGHT_CHARS is returned as plain text.
export const MAX_HIGHLIGHT_CHARS = 60000;

const kw = (s) => new Set(s.split(' '));

const C_LIKE = { line: ['//'], block: [['/*', '*/']], quotes: '"\'' };

const LANGS = {
  go: { ...C_LIKE, quotes: '"\'`', multi: '`', keywords: kw('break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var true false nil iota string int int64 uint8 byte rune error bool any make new len cap append panic recover') },
  javascript: { ...C_LIKE, quotes: '"\'`', multi: '`', keywords: kw('async await break case catch class const continue debugger default delete do else export extends finally for from function if import in instanceof let new of return static super switch this throw try typeof var void while with yield true false null undefined') },
  typescript: { ...C_LIKE, quotes: '"\'`', multi: '`', keywords: kw('async await break case catch class const continue default delete do else enum export extends finally for from function if implements import in instanceof interface let new of private protected public readonly return static super switch this throw try type typeof var void while yield true false null undefined string number boolean any unknown never') },
  python: { line: ['#'], quotes: '"\'', triple: true, keywords: kw('and as assert async await break class continue def del elif else except finally for from global if import in is lambda None nonlocal not or pass raise return try while with yield True False self') },
  bash: { line: ['#'], quotes: '"\'', multi: '"\'', vars: true, keywords: kw('if then else elif fi for while until do done case esac in function select time export local readonly declare unset return exit set shift source alias echo cd') },
  sql: { line: ['--'], block: [['/*', '*/']], quotes: '\'"', ci: true, keywords: kw('select from where and or not in is null like insert into values update set delete create table alter drop index view join inner left right full outer on group by order having limit offset union all distinct as case when then else end primary key foreign references default unique grant revoke begin commit rollback with exists between asc desc') },
  json: { quotes: '"', jsonKeys: true, keywords: kw('true false null') },
  yaml: {
    line: ['#'], quotes: '"\'', keywords: kw('true false null yes no on off'),
    lineRules: [[/[ \t]*(?:- +)?[A-Za-z0-9_.\-/]+(?=[ \t]*:(?:[ \t]|$))/y, 'tk-a']],
  },
  toml: {
    line: ['#'], quotes: '"\'', triple: true, keywords: kw('true false'),
    lineRules: [[/[ \t]*\[\[?[^\]\n]*\]\]?/y, 'tk-t'], [/[ \t]*[A-Za-z0-9_.\-]+(?=[ \t]*=)/y, 'tk-a']],
  },
  dockerfile: {
    line: ['#'], quotes: '"\'', vars: true, keywords: new Set(),
    lineRules: [[/[ \t]*(?:FROM|RUN|CMD|LABEL|MAINTAINER|EXPOSE|ENV|ADD|COPY|ENTRYPOINT|VOLUME|USER|WORKDIR|ARG|ONBUILD|STOPSIGNAL|HEALTHCHECK|SHELL|AS)\b/iy, 'tk-k']],
  },
  html: { markup: true },
  xml: { markup: true },
};

const NUM_RE = /0[xX][0-9a-fA-F_]+|\d[\d_]*(?:\.\d+)?(?:[eE][+-]?\d+)?/y;
const IDENT_RE = /[A-Za-z_][\w]*/y;
const VAR_RE = /\$(?:\{[^}\n]*\}|[A-Za-z_]\w*|[0-9@#?*!$-])/y;
const WORD = /[\w$]/;

function sticky(re, s, i) { re.lastIndex = i; const m = re.exec(s); return m && m.index === i ? m[0] : null; }

function scanString(text, i, def) {
  const q = text[i];
  const n = text.length;
  if (def.triple && text.startsWith(q + q + q, i)) {
    const end = text.indexOf(q + q + q, i + 3);
    return end < 0 ? n : end + 3;
  }
  const multi = def.multi && def.multi.includes(q);
  let j = i + 1;
  while (j < n) {
    const c = text[j];
    if (c === '\\' && !(def.vars && q === '\'')) { j += 2; continue; }
    if (c === q) return j + 1;
    if (c === '\n' && !multi) return j;
    j++;
  }
  return n;
}

function scanTag(text, i) {
  // returns list of [cls, str] for a tag starting at text[i] === '<'
  const n = text.length;
  const parts = [];
  let j = i;
  let start = i;
  const flushTag = (end) => { if (end > start) parts.push(['tk-t', text.slice(start, end)]); };
  while (j < n) {
    const c = text[j];
    if (c === '"' || c === '\'') {
      flushTag(j);
      let e = text.indexOf(c, j + 1);
      e = e < 0 ? n : e + 1;
      parts.push(['tk-s', text.slice(j, e)]);
      j = e; start = j;
      continue;
    }
    j++;
    if (c === '>') break;
  }
  flushTag(j);
  return { parts, end: j };
}

const ALIASES = { js: 'javascript', ts: 'typescript', py: 'python', sh: 'bash', shell: 'bash', zsh: 'bash', yml: 'yaml', golang: 'go', docker: 'dockerfile', htm: 'html' };

export function tokenize(text, lang) {
  const def = LANGS[ALIASES[lang] || lang];
  if (!def || text.length > MAX_HIGHLIGHT_CHARS) return [['', text]];
  const out = [];
  const add = (cls, s) => {
    if (!s) return;
    const last = out[out.length - 1];
    if (last && last[0] === cls) last[1] += s; else out.push([cls, s]);
  };
  const n = text.length;
  let i = 0;
  let plainStart = 0;
  const flushPlain = (upto) => { if (upto > plainStart) add('', text.slice(plainStart, upto)); };
  const tok = (cls, end) => { flushPlain(i); add(cls, text.slice(i, end)); i = end; plainStart = i; };

  outer:
  while (i < n) {
    const c = text[i];
    if (def.markup) {
      if (text.startsWith('<!--', i)) { const e = text.indexOf('-->', i + 4); tok('tk-c', e < 0 ? n : e + 3); continue; }
      if (c === '<' && /[A-Za-z/!?]/.test(text[i + 1] || '')) {
        flushPlain(i);
        const { parts, end } = scanTag(text, i);
        for (const [cls, s] of parts) add(cls, s);
        i = end; plainStart = i;
        continue;
      }
      i++;
      continue;
    }
    if ((i === 0 || text[i - 1] === '\n') && def.lineRules) {
      for (const [re, cls] of def.lineRules) {
        const m = sticky(re, text, i);
        if (m) { tok(cls, i + m.length); continue outer; }
      }
    }
    if (def.line) {
      for (const p of def.line) {
        if (text.startsWith(p, i)) {
          let e = text.indexOf('\n', i);
          if (e < 0) e = n;
          tok('tk-c', e);
          continue outer;
        }
      }
    }
    if (def.block) {
      for (const [s, e] of def.block) {
        if (text.startsWith(s, i)) { const x = text.indexOf(e, i + s.length); tok('tk-c', x < 0 ? n : x + e.length); continue outer; }
      }
    }
    if (def.quotes && def.quotes.includes(c)) {
      const e = scanString(text, i, def);
      let cls = 'tk-s';
      if (def.jsonKeys && /^\s*:/.test(text.slice(e, e + 20))) cls = 'tk-a';
      tok(cls, e);
      continue;
    }
    if (def.vars && c === '$') {
      const m = sticky(VAR_RE, text, i);
      if (m) { tok('tk-v', i + m.length); continue; }
    }
    if (c >= '0' && c <= '9' && !(i > 0 && WORD.test(text[i - 1]))) {
      const m = sticky(NUM_RE, text, i);
      if (m) { tok('tk-n', i + m.length); continue; }
    }
    if (/[A-Za-z_]/.test(c) && !(i > 0 && WORD.test(text[i - 1]))) {
      const m = sticky(IDENT_RE, text, i);
      if (m) {
        if (def.keywords.has(def.ci ? m.toLowerCase() : m)) tok('tk-k', i + m.length);
        else { i += m.length; }
        continue;
      }
    }
    i++;
  }
  flushPlain(n);
  return out;
}

export function highlightInto(doc, el, text, lang) {
  for (const [cls, s] of tokenize(text, lang)) {
    if (!cls) { el.appendChild(doc.createTextNode(s)); continue; }
    const span = doc.createElement('span');
    span.setAttribute('class', cls);
    span.appendChild(doc.createTextNode(s));
    el.appendChild(span);
  }
}
