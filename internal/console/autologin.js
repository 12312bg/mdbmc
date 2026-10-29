// Injected into every document of a console window. It only acts on the
// BMC's pages (served through the window's proxy): it fills the login form
// once, then optionally opens the page given as "after" (e.g. the KVM
// viewer) once the login went through. It reports what it finds and does
// to the log, describing elements (tag, type, id, name), never their values.
(function (cfg) {
  if (location.host.toLowerCase() !== cfg.host) return;
  var KEY = '__mdbmc_' + cfg.nonce;
  // Progress reports to the window process (logged, and used by tests).
  // Frames of the same page post through the top window.
  function report(name, detail) {
    try { window.top.chrome.webview.postMessage('mdbmc:' + name + '\t' + String(detail || '')); } catch (e) {}
  }
  function get() { try { return sessionStorage.getItem(KEY); } catch (e) { return null; } }
  function set(v) { try { sessionStorage.setItem(KEY, v); } catch (e) {} }
  function visible(el) { return !!(el && (el.offsetWidth || el.offsetHeight || el.getClientRects().length)); }
  function passwordField() {
    var all = document.querySelectorAll('input[type=password]');
    for (var i = 0; i < all.length; i++) if (visible(all[i]) && !all[i].disabled) return all[i];
    return null;
  }
  // desc names an element for the log, e.g. button[type=submit]#login "登录".
  function desc(el) {
    if (!el) return 'none';
    var s = el.tagName.toLowerCase();
    var type = el.getAttribute('type');
    if (type) s += '[type=' + type + ']';
    if (el.id) s += '#' + el.id;
    var name = el.getAttribute('name');
    if (name) s += '[name=' + name + ']';
    var label = el.tagName === 'INPUT' ? (/^(button|submit)$/i.test(type || '') ? el.value : '') : el.innerText;
    label = (label || '').trim().slice(0, 20);
    if (label) s += ' "' + label + '"';
    if (el.disabled) s += ' (disabled)';
    if (!visible(el)) s += ' (hidden)';
    return s.slice(0, 160);
  }
  // inventory summarizes the page when no login form was found.
  function inventory() {
    var counts = {};
    var inputs = document.querySelectorAll('input');
    for (var i = 0; i < inputs.length; i++) {
      var t = (inputs[i].getAttribute('type') || 'text').toLowerCase();
      var c = counts[t] || (counts[t] = [0, 0]);
      c[0]++;
      if (visible(inputs[i])) c[1]++;
    }
    var parts = [];
    for (var k in counts) parts.push(k + ' ' + counts[k][1] + '/' + counts[k][0] + ' visible');
    return 'inputs: ' + (parts.join(', ') || 'none') + '; forms: ' + document.forms.length +
      '; frames: ' + document.querySelectorAll('iframe,frame').length;
  }
  function setValue(el, v) {
    var setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
    setter.call(el, v);
    // Frameworks listen to different events before they accept the value
    // or enable the login button.
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
    el.dispatchEvent(new KeyboardEvent('keyup', { bubbles: true }));
    el.dispatchEvent(new Event('blur'));
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
  function fill(pw) {
    var user = userField(pw);
    if (user) setValue(user, cfg.user);
    setValue(pw, cfg.pass);
    return user;
  }
  function submit(pw) {
    var scope = pw.form || document;
    var btn = scope.querySelector('button[type=submit],input[type=submit]');
    if (!btn) {
      var cands = document.querySelectorAll('button,input[type=button],a,[role=button]');
      for (var i = 0; i < cands.length; i++) {
        var txt = (cands[i].innerText || cands[i].value || '').trim();
        if (visible(cands[i]) && /^(log ?in|sign ?in|登\s*录|登入)$/i.test(txt)) { btn = cands[i]; break; }
      }
    }
    if (btn) { report('autologin', 'click ' + desc(btn)); btn.click(); return; }
    if (pw.form) {
      report('autologin', 'no login button, submitting the form');
      if (pw.form.requestSubmit) pw.form.requestSubmit(); else pw.form.submit();
      return;
    }
    report('autologin', 'no login button or form, pressing Enter');
    ['keydown', 'keypress', 'keyup'].forEach(function (type) {
      pw.dispatchEvent(new KeyboardEvent(type, { key: 'Enter', keyCode: 13, which: 13, bubbles: true }));
    });
  }
  // settle submits once the form is stable: pages that render their form in
  // steps may replace or clear it right after it was filled.
  function settle(pw, n) {
    var cur = passwordField();
    if (cur && (cur !== pw || cur.value !== cfg.pass) && n < 3) {
      report('autologin', 'form was re-rendered or cleared, filling again');
      fill(cur);
      setTimeout(function () { settle(cur, n + 1); }, 400);
      return;
    }
    submit(cur || pw);
    report('submitted', location.pathname);
    waitLoggedIn(true);
  }
  // goAfter opens the page to show after the login (e.g. the KVM), once.
  function goAfter(why) {
    if (!cfg.after || get() === 'redirected') return;
    set('redirected');
    report('autologin', why + ', opening ' + cfg.after.replace(/^[a-z]+:\/\/[^\/]+/i, '').split('?')[0]);
    location.href = cfg.after;
  }
  // waitLoggedIn waits for the login form to go away (single-page UIs log
  // in without a reload), then goes on to "after". It keeps waiting after
  // a failed automatic login, so that logging in by hand still gets there.
  function waitLoggedIn(submitted) {
    var tries = 0;
    var t = setInterval(function () {
      tries++;
      if (!passwordField()) {
        clearInterval(t);
        goAfter('signed in');
      } else if (tries === 60 && submitted) {
        report('autologin', 'still on the login page 30s after submitting (wrong password, captcha or unsupported form?)');
        if (!cfg.after) clearInterval(t);
      } else if (tries > 1200) {
        clearInterval(t);
      }
    }, 500);
  }
  // waitForForm waits for a login form that this script does not fill in.
  function waitForForm(n) {
    if (passwordField()) waitLoggedIn(false);
    else if (n < 1200) setTimeout(function () { waitForForm(n + 1); }, 500);
  }
  function attempt(n) {
    var state = get();
    if (state === 'redirected') return;
    if (state === 'submitted') { waitLoggedIn(false); return; }
    var pw = passwordField();
    if (!pw) {
      // No login form once the page has settled: the BMC still has a
      // session for this window's profile.
      if (cfg.after && n >= 20 && document.readyState === 'complete') { goAfter('no login form, already signed in'); return; }
      if (n < 60) setTimeout(function () { attempt(n + 1); }, 500);
      else report('autologin', 'no visible password field after 30s; ' + inventory());
      return;
    }
    var user = fill(pw);
    report('autologin', 'filled user=' + desc(user) + ' password=' + desc(pw));
    set('submitted');
    setTimeout(function () { settle(pw, 0); }, 300);
  }
  function startFrame() {
    // Only the top page is filled in; say so when the login form is in a frame.
    if (!cfg.autologin) return;
    setTimeout(function () {
      if (passwordField()) report('autologin', 'login form is inside a frame (' + location.pathname + '), not filled');
    }, 2000);
  }
  function start() {
    // The proxy's own error pages carry this marker; there is nothing to log in to.
    if (document.querySelector('meta[name="mdbmc-status"]')) { report('status-page', location.pathname); return; }
    report('loaded', location.pathname);
    if (!cfg.autologin) {
      report('autologin', 'off for this account');
      if (cfg.after) waitForForm(0);
      return;
    }
    attempt(0);
  }
  var run = window.top === window ? start : startFrame;
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', run);
  else run();
})(__MDBMC_CONFIG__);
