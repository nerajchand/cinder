// Minimal DOM stand-in so renderers can be tested without a browser.
class Node_ {
  constructor(tag) { this.tag = tag; this.children = []; this.attrs = {}; this.text = null; }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  appendChild(c) { if (c.tag === '#frag') this.children.push(...c.children); else this.children.push(c); return c; }
  get firstChild() { return this.children[0] || null; }
  removeChild(c) { this.children.splice(this.children.indexOf(c), 1); }
}
export const doc = {
  createElement: (t) => new Node_(t),
  createTextNode: (s) => { const n = new Node_('#text'); n.text = s; return n; },
  createDocumentFragment: () => new Node_('#frag'),
};
export function walk(n, fn) { fn(n); n.children.forEach((c) => walk(c, fn)); }
export function find(n, tag) { const r = []; walk(n, (x) => { if (x.tag === tag) r.push(x); }); return r; }
export function textOf(n) { let s = ''; walk(n, (x) => { if (x.text != null) s += x.text; }); return s; }
