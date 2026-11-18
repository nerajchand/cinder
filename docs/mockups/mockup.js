/* Mockup-only behaviour. Theme: dark by default; toggle switches to light. */
(function () {
  var root = document.documentElement;
  var saved = null;
  try { saved = localStorage.getItem('cinder-theme'); } catch (e) {}
  if (saved === 'light') root.setAttribute('data-theme', 'light');
  var t = document.querySelector('[data-theme-toggle]');
  function label() { t.textContent = root.getAttribute('data-theme') === 'light' ? 'Switch to dark' : 'Switch to light'; }
  if (t) {
    t.hidden = false; label();
    t.addEventListener('click', function () {
      var light = root.getAttribute('data-theme') !== 'light';
      if (light) root.setAttribute('data-theme', 'light'); else root.removeAttribute('data-theme');
      try { localStorage.setItem('cinder-theme', light ? 'light' : 'dark'); } catch (e) {}
      label();
    });
  }
  var ta = document.getElementById('secret-input'), cnt = document.getElementById('counter');
  if (ta && cnt) {
    var max = 65536;
    var upd = function () {
      var n = new Blob([ta.value]).size;
      cnt.textContent = n.toLocaleString() + ' / 65,536 bytes (64 KiB)';
      cnt.style.color = n > max ? 'var(--danger)' : '';
    };
    ta.addEventListener('input', upd); upd();
  }
  var tog = document.querySelector('[data-toggle-secret]');
  if (tog) {
    var box = document.getElementById('secret-value');
    var real = box.getAttribute('data-real'), mask = '\u2022'.repeat(24);
    tog.addEventListener('click', function () {
      var shown = tog.getAttribute('aria-pressed') === 'true';
      tog.setAttribute('aria-pressed', String(!shown));
      tog.textContent = shown ? 'Show' : 'Hide';
      box.textContent = shown ? mask : real;
    });
  }
  document.querySelectorAll('[data-copy]').forEach(function (b) {
    b.addEventListener('click', function () {
      var s = document.getElementById('toast'); if (s) s.textContent = 'Copied to clipboard (mock).';
    });
  });
})();
