// Injected into every document of a console window. It only acts on the
// BMC's pages (served through the window's proxy): it fills the login form once, then optionally opens the
// page given as "after" (e.g. the KVM viewer) once the login went through.
(function (cfg) {
  if (window.top !== window || location.host.toLowerCase() !== cfg.host) return;
  var KEY = '__mdbmc_' + cfg.nonce;
  // Progress reports to the window process (logged, and used by tests).
  function report(name, detail) {
    try { window.chrome.webview.postMessage('mdbmc:' + name + '\t' + String(detail || '')); } catch (e) {}
  }
  function get() { try { return sessionStorage.getItem(KEY); } catch (e) { return null; } }
  function set(v) { try { sessionStorage.setItem(KEY, v); } catch (e) {} }
  function visible(el) { return !!(el && (el.offsetWidth || el.offsetHeight || el.getClientRects().length)); }
  function passwordField() {
    var all = document.querySelectorAll('input[type=password]');
    for (var i = 0; i < all.length; i++) if (visible(all[i]) && !all[i].disabled) return all[i];
    return null;
  }
  function setValue(el, v) {
    var desc = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value');
    desc.set.call(el, v);
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
  }
  function userField(pw) {
    var inputs = document.querySelectorAll('input');
    var best = null;
    for (var i = 0; i < inputs.length; i++) {
      var e = inputs[i];
      var t = (e.getAttribute('type') || 'text').toLowerCase();
      if (!visible(e) || (t !== 'text' && t !== 'email')) continue;
      if (e.compareDocumentPosition(pw) & Node.DOCUMENT_POSITION_FOLLOWING) best = e;
      else if (!best) best = e;
    }
    return best;
  }
  function submit(pw) {
    var scope = pw.form || document;
    var btn = scope.querySelector('button[type=submit],input[type=submit]');
    if (!btn) {
      var cands = document.querySelectorAll('button,input[type=button],a,[role=button]');
      for (var i = 0; i < cands.length; i++) {
        var txt = (cands[i].innerText || cands[i].value || '').trim();
        if (visible(cands[i]) && /^(log ?in|sign ?in|登录|登 录|登入)$/i.test(txt)) { btn = cands[i]; break; }
      }
    }
    if (btn) { btn.click(); return; }
    if (pw.form) { if (pw.form.requestSubmit) pw.form.requestSubmit(); else pw.form.submit(); return; }
    pw.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', keyCode: 13, which: 13, bubbles: true }));
  }
  function afterLogin() {
    // Single-page UIs log in without a reload: wait for the form to go away.
    var tries = 0;
    var t = setInterval(function () {
      tries++;
      if (!passwordField()) {
        clearInterval(t);
        if (cfg.after && get() !== 'redirected') { set('redirected'); location.href = cfg.after; }
      } else if (tries > 60) {
        clearInterval(t); // login failed or needs a manual step; leave the page to the user
      }
    }, 500);
  }
  function attempt(n) {
    var state = get();
    if (state === 'redirected') return;
    if (state === 'submitted') { afterLogin(); return; }
    if (!cfg.autologin) return;
    var pw = passwordField();
    if (!pw) { if (n < 60) setTimeout(function () { attempt(n + 1); }, 500); return; }
    var user = userField(pw);
    if (user) setValue(user, cfg.user);
    setValue(pw, cfg.pass);
    set('submitted');
    setTimeout(function () { submit(pw); report('submitted', location.pathname); afterLogin(); }, 300);
  }
  function start() {
    // The proxy's own error pages carry this marker; there is nothing to log in to.
    if (document.querySelector('meta[name="mdbmc-status"]')) { report('status-page', location.pathname); return; }
    report('loaded', location.pathname);
    attempt(0);
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})(__MDBMC_CONFIG__);
