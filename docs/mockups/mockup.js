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
  var tabs = document.querySelectorAll('[role=tab]');
  tabs.forEach(function (tab) {
    tab.addEventListener('click', function () {
      tabs.forEach(function (o) {
        var on = o === tab;
        o.setAttribute('aria-selected', String(on));
        o.tabIndex = on ? 0 : -1;
        document.getElementById(o.getAttribute('aria-controls')).hidden = !on;
      });
    });
    tab.addEventListener('keydown', function (e) {
      if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
      var list = Array.prototype.slice.call(tabs);
      var n = list[(list.indexOf(tab) + (e.key === 'ArrowRight' ? 1 : list.length - 1)) % list.length];
      n.focus(); n.click();
    });
  });
  var lang = document.getElementById('lang');
  var lb = document.getElementById('lang-badge');
  if (lang && lb) lang.addEventListener('change', function () { lb.textContent = lang.options[lang.selectedIndex].text; });
  var tog = document.querySelector('[data-toggle-secret]');
  if (tog) {
    var mask = document.getElementById('secret-mask'), real = document.getElementById('secret-rendered');
    tog.addEventListener('click', function () {
      var shown = tog.getAttribute('aria-pressed') === 'true';
      tog.setAttribute('aria-pressed', String(!shown));
      tog.textContent = shown ? 'Show' : 'Hide';
      mask.hidden = !shown; real.hidden = shown;
    });
  }
  document.querySelectorAll('[data-copy]').forEach(function (b) {
    b.addEventListener('click', function () {
      var s = document.getElementById('toast'); if (s) s.textContent = 'Copied to clipboard (mock).';
    });
  });
})();
