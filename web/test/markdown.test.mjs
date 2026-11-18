import test from 'node:test';
import assert from 'node:assert/strict';
import { parseMarkdown, parseInline, renderMarkdown, safeHref } from '../../internal/web/dist/static/markdown.js';
import { tokenize, MAX_HIGHLIGHT_CHARS } from '../../internal/web/dist/static/highlight.js';
import { doc, find, textOf, walk } from './fakedom.mjs';

const render = (s) => renderMarkdown(s, doc);
const tags = (n) => { const t = new Set(); walk(n, (x) => t.add(x.tag)); return t; };

test('script tags are inert text, never elements', () => {
  const out = render('<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>');
  const t = tags(out);
  assert.ok(!t.has('script') && !t.has('img'));
  assert.ok(textOf(out).includes('<script>alert(1)</script>'));
  assert.ok(textOf(out).includes('<img src=x onerror=alert(1)>'));
});

test('only http, https and mailto links become anchors', () => {
  const bad = ['javascript:alert(1)', 'JaVaScRiPt:alert(1)', 'java\tscript:alert(1)', ' javascript:alert(1)', 'data:text/html,hi', 'vbscript:x', 'file:///etc/passwd', '//evil.example', '/relative', '#frag'];
  for (const u of bad) {
    const out = render(`[click](${u})`);
    assert.equal(find(out, 'a').length, 0, u);
  }
  for (const u of ['http://a.example/x', 'https://a.example/?q=1', 'mailto:me@example.com']) {
    const a = find(render(`[go](${u})`), 'a');
    assert.equal(a.length, 1, u);
    assert.equal(a[0].attrs.rel, 'noopener noreferrer nofollow');
    assert.ok(/^(https?|mailto):/.test(a[0].attrs.href));
  }
});

test('rejected link keeps its destination visible as text', () => {
  const out = render('[click](javascript:alert(1))');
  assert.ok(textOf(out).includes('click'));
  assert.ok(textOf(out).includes('javascript:alert(1)'));
});

test('images are never rendered; shown as text', () => {
  const out = render('![alt text](https://evil.example/track.png)');
  assert.ok(!tags(out).has('img'));
  assert.equal(textOf(out), '![alt text](https://evil.example/track.png)');
  assert.equal(find(out, 'a').length, 0);
});

test('no attribute other than href/rel/target/class is ever set; no event handlers', () => {
  const src = '# h\n\n**b** *i* `c` [l](https://x.example "t")\n\n> q\n\n- a\n- b\n\n1. x\n\n```js\nconst a = 1;\n```\n';
  const out = render(src);
  walk(out, (n) => { for (const k of Object.keys(n.attrs)) assert.ok(['href', 'rel', 'target', 'class'].includes(k), k); });
});

test('block and inline structure', () => {
  const ast = parseMarkdown('# Title\n\nSome **bold**, *em* and `code`.\n\n- one\n- two\n  - nested\n\n1. a\n2. b\n\n> quote\n\n---\n\n```go\nfmt.Println()\n```');
  assert.deepEqual(ast.map((b) => b.type), ['heading', 'paragraph', 'list', 'list', 'blockquote', 'hr', 'code']);
  assert.equal(ast[2].items.length, 2);
  assert.equal(ast[2].items[1][1].type, 'list');
  assert.equal(ast[3].ordered, true);
  assert.equal(ast[6].lang, 'go');
  assert.equal(ast[6].text, 'fmt.Println()');
  const out = render('Some **bold**, *em* and `code`.');
  assert.deepEqual([...tags(out)].sort(), ['#frag', '#text', 'code', 'em', 'p', 'strong']);
});

test('code fence content is literal', () => {
  const out = render('```\n**not bold** <b>x</b>\n```');
  assert.ok(!tags(out).has('strong') && !tags(out).has('b'));
  assert.ok(textOf(out).includes('**not bold** <b>x</b>'));
});

test('unterminated constructs do not throw or hang', () => {
  for (const s of ['**', '*a', '[a](', '[a](b', '`x', '```\nopen', '> > > > > > > > > > > x', '- - - - - - - - - - - x', '![', '[[[[[[']) {
    assert.doesNotThrow(() => render(s), s);
  }
});

test('hostile 64 KiB input renders in bounded time', () => {
  for (const s of ['*a '.repeat(30000), '[a'.repeat(30000), '`'.repeat(60000), '_x'.repeat(30000), '- '.repeat(30000), '> '.repeat(30000), '**a '.repeat(16000)]) {
    const t0 = Date.now();
    render(s);
    assert.ok(Date.now() - t0 < 3000, `slow: ${s.slice(0, 6)}`);
  }
});

test('safeHref normalises and rejects', () => {
  assert.equal(safeHref('https://a.example'), 'https://a.example/');
  assert.equal(safeHref('javascript:1'), null);
  assert.equal(safeHref(''), null);
  assert.deepEqual(parseInline('plain').map((n) => n.type), ['text']);
});

test('highlighter: classes only, bounded, reconstructs input', () => {
  const samples = {
    go: 'package main\n// hi\nfunc main() { x := "s"; y := 42 }',
    json: '{"a": [1, true, null], "b": "c"}',
    yaml: '# c\nkey: value\nlist:\n  - a: 1',
    bash: 'echo "$HOME" # c\nexport A=${B}',
    html: '<!-- c --><a href="x">t</a>',
    sql: "SELECT * FROM t WHERE a = 'x' -- c",
    dockerfile: 'FROM alpine\nRUN echo hi',
    toml: '[sec]\nk = "v"\nn = 1',
    python: 'def f():\n    """doc"""\n    return None',
  };
  for (const [lang, src] of Object.entries(samples)) {
    const toks = tokenize(src, lang);
    assert.equal(toks.map((t) => t[1]).join(''), src, lang);
    assert.ok(toks.some((t) => t[0]), `${lang} produced no classed tokens`);
    for (const [cls] of toks) assert.match(cls, /^(tk-[a-z])?$/);
  }
  assert.deepEqual(tokenize('{"a":1}', 'json').map((t) => t[0]), ['', 'tk-a', '', 'tk-n', '']);
  assert.deepEqual(tokenize('x', 'nonsense'), [['', 'x']]);
  const big = 'a'.repeat(MAX_HIGHLIGHT_CHARS + 1);
  assert.deepEqual(tokenize(big, 'go'), [['', big]]);
  const t0 = Date.now();
  tokenize('"'.repeat(MAX_HIGHLIGHT_CHARS) , 'go');
  tokenize('/*'.repeat(MAX_HIGHLIGHT_CHARS / 2), 'go');
  assert.ok(Date.now() - t0 < 2000);
});
