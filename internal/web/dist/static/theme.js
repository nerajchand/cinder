// Classic (non-module) script loaded in <head> so the stored theme applies before paint.
// Dark is the default; 'light' is persisted in localStorage.
(function () {
  var root = document.documentElement;
  var KEY = 'cinder-theme';
  try { if (localStorage.getItem(KEY) === 'light') root.setAttribute('data-theme', 'light'); } catch (e) { /* storage blocked */ }
  document.addEventListener('DOMContentLoaded', function () {
    var t = document.querySelector('[data-theme-toggle]');
    if (!t) return;
    function label() { t.textContent = root.getAttribute('data-theme') === 'light' ? 'Switch to dark' : 'Switch to light'; }
    t.hidden = false;
    label();
    t.addEventListener('click', function () {
      var light = root.getAttribute('data-theme') !== 'light';
      if (light) root.setAttribute('data-theme', 'light'); else root.removeAttribute('data-theme');
      try { localStorage.setItem(KEY, light ? 'light' : 'dark'); } catch (e) { /* ignore */ }
      label();
    });
  });
})();
