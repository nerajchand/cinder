import test from 'node:test';
import assert from 'node:assert/strict';
import { encodeText, numDataCodewords, rsRemainder, interleave } from '../../internal/web/dist/static/vendor/qrcode.js';

test('data capacity table matches ISO 18004 (spot checks)', () => {
  const L = { 1: 19, 5: 108, 10: 274, 20: 861, 40: 2956 };
  const M = { 1: 16, 5: 86, 10: 216, 20: 669, 40: 2334 };
  for (const [v, n] of Object.entries(L)) assert.equal(numDataCodewords(+v, 'L'), n, `L${v}`);
  for (const [v, n] of Object.entries(M)) assert.equal(numDataCodewords(+v, 'M'), n, `M${v}`);
  for (let v = 2; v <= 40; v++) {
    assert.ok(numDataCodewords(v, 'L') > numDataCodewords(v - 1, 'L'));
    assert.ok(numDataCodewords(v, 'M') > numDataCodewords(v - 1, 'M'));
    assert.ok(numDataCodewords(v, 'L') > numDataCodewords(v, 'M'));
  }
});

test('format information for known (level, mask) pairs', () => {
  // Read the 15 format bits next to the top-left finder: bits 0-5 at (8,0..5), 6 at (8,7), 7 at (8,8), 8 at (7,8), 9-14 at (14-i,8).
  const read = (q) => {
    let v = 0;
    const pos = [[8, 0], [8, 1], [8, 2], [8, 3], [8, 4], [8, 5], [8, 7], [8, 8], [7, 8], [5, 8], [4, 8], [3, 8], [2, 8], [1, 8], [0, 8]];
    pos.forEach(([x, y], i) => { if (q.get(x, y)) v |= 1 << i; });
    return v;
  };
  assert.equal(read(encodeText('hi', { ecl: 'L', mask: 0 })), 0b111011111000100); // L, mask 0
  assert.equal(read(encodeText('hi', { ecl: 'M', mask: 0 })), 0b101010000010010); // M, mask 0
});

test('structure: size, finder patterns, timing, dark module', () => {
  const q = encodeText('https://cinder.example/s/abc#key');
  assert.equal(q.size, q.version * 4 + 17);
  for (const [ox, oy] of [[0, 0], [q.size - 7, 0], [0, q.size - 7]]) {
    for (let d = 0; d < 7; d++) {
      assert.ok(q.get(ox + d, oy) && q.get(ox + d, oy + 6) && q.get(ox, oy + d) && q.get(ox + 6, oy + d));
    }
    assert.ok(q.get(ox + 3, oy + 3) && !q.get(ox + 1, oy + 1));
  }
  for (let i = 8; i < q.size - 8; i++) assert.equal(q.get(i, 6), i % 2 === 0);
  assert.ok(q.get(8, q.size - 8));
});

test('reed-solomon: single-block codeword has zero syndromes; interleave length is right', () => {
  const mul = (x, y) => { let z = 0; for (let i = 7; i >= 0; i--) { z = (z << 1) ^ ((z >>> 7) * 0x11d); z ^= ((y >>> i) & 1) * x; } return z; };
  // Version 1-L is one block: 19 data + 7 ecc codewords. c(alpha^i) must be 0 for i = 0..6.
  const data = Array.from({ length: 19 }, (_, i) => (i * 29 + 7) & 255);
  const cw = interleave(data, 1, 'L');
  assert.equal(cw.length, 26);
  assert.deepEqual(cw.slice(0, 19), data);
  let root = 1;
  for (let i = 0; i < 7; i++) {
    let acc = 0;
    for (const c of cw) acc = mul(acc, root) ^ c; // Horner
    assert.equal(acc, 0, `syndrome ${i}`);
    root = mul(root, 2);
  }
  assert.equal(rsRemainder(data, [1]).length, 1);
  for (const [v, l] of [[1, 'L'], [5, 'M'], [10, 'L'], [20, 'M'], [40, 'L']]) {
    const a = Math.floor(v / 7) + 2;
    const raw = Math.floor(((16 * v + 128) * v + 64 - (v >= 2 ? (25 * a - 10) * a - 55 + (v >= 7 ? 36 : 0) : 0)) / 8);
    assert.equal(interleave(new Array(numDataCodewords(v, l)).fill(0x55), v, l).length, raw, `${l}${v}`);
  }
});

test('typical share link fits a small version; huge input throws', () => {
  const link = `https://cinder.example.com/s/${'a'.repeat(22)}#${'b'.repeat(43)}`;
  const q = encodeText(link);
  assert.ok(q.version <= 6, `version ${q.version}`);
  assert.throws(() => encodeText('x'.repeat(3000)));
});
