package console

import (
	"bytes"
	"html/template"
)

// page is a status page shown in a console window in place of a blank
// WebView: while the BMC is being reached, when it cannot be, and while a
// Java console starts.
type page struct {
	// Tone is "loading", "error", "warning" or "java"; a "java" page follows
	// the Java console's progress through javaStatusPath.
	Tone    string
	Title   string
	Message string
	Target  string   // BMC address
	Hints   []string // what the user can try
	Detail  string   // technical detail, collapsed
	Retry   string   // URL behind the "reconnect" button
}

// openMessage is posted by the loading page to have the window open the BMC.
const openMessage = "mdbmc:open"

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="mdbmc-status" content="{{.Tone}}">
<title>{{.Title}}</title>
<style>
:root{color-scheme:light;--bg:#f4f6f9;--surface:#fff;--border:#e6e8ec;--text-1:#14171c;--text-2:#4b5563;--text-3:#8a93a0;--accent:#2a78d6;--accent-soft:#e8f1fc;--danger:#d03b3b;--danger-soft:#fdeeee;--warning:#b86e00;--warning-soft:#fff4dc;--ok:#0c8a0c;--ok-soft:#e6f5e6;--code:#f7f8fa}
@media (prefers-color-scheme:dark){:root{color-scheme:dark;--bg:#111215;--surface:#1a1b1f;--border:#2c2f36;--text-1:#eef0f3;--text-2:#b6bcc6;--text-3:#7c8491;--accent:#3987e5;--accent-soft:#1d2a3b;--danger:#ef6b6b;--danger-soft:#3a1f22;--warning:#f0b43c;--warning-soft:#3a3020;--ok:#3fbf5a;--ok-soft:#1c3122;--code:#202227}}
*{box-sizing:border-box}
html,body{height:100%;margin:0}
body{display:flex;align-items:center;justify-content:center;padding:24px 16px;background:var(--bg);color:var(--text-1);font:14px/1.65 "Segoe UI","Microsoft YaHei UI","Microsoft YaHei","PingFang SC",system-ui,sans-serif}
.card{width:100%;max-width:560px;background:var(--surface);border:1px solid var(--border);border-radius:12px;padding:30px 36px 28px;box-shadow:0 1px 2px rgba(16,24,40,.04),0 8px 24px rgba(16,24,40,.06)}
.brand{display:flex;align-items:center;gap:8px;font-size:12px;color:var(--text-3)}
.brand i{width:8px;height:8px;border-radius:2px;background:var(--accent)}
.icon{margin-top:22px;width:48px;height:48px;border-radius:12px;display:flex;align-items:center;justify-content:center}
.error .icon{background:var(--danger-soft);color:var(--danger)}
.warning .icon{background:var(--warning-soft);color:var(--warning)}
.loading .icon,.java .icon{background:var(--accent-soft);color:var(--accent)}
.java.ok .icon{background:var(--ok-soft);color:var(--ok)}
.java.fail .icon{background:var(--danger-soft);color:var(--danger)}
.java .icon>*{display:none}
.java .i-wait,.java.ok .i-ok,.java.fail .i-fail{display:block}
.java.ok .i-wait,.java.fail .i-wait{display:none}
#status{margin-top:14px;padding:10px 12px;border-radius:8px;background:var(--code);color:var(--text-2);font-size:13px;white-space:pre-wrap;word-break:break-word}
.java.fail #status{color:var(--danger)}
.spin{width:22px;height:22px;border:3px solid currentColor;border-right-color:transparent;border-radius:50%;animation:spin .8s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}
h1{margin:16px 0 6px;font-size:18px;font-weight:600}
p{margin:0;color:var(--text-2)}
.target{display:inline-block;margin-top:12px;padding:1px 10px;border-radius:999px;background:var(--code);border:1px solid var(--border);font:12px/20px "Cascadia Mono",Consolas,monospace;color:var(--text-2)}
ul{margin:16px 0 0;padding-left:18px;color:var(--text-2)}
li+li{margin-top:4px}
details{margin-top:16px}
summary{cursor:pointer;color:var(--text-3);font-size:12px}
pre{margin:8px 0 0;padding:10px 12px;border-radius:8px;background:var(--code);color:var(--text-2);font:12px/1.5 "Cascadia Mono",Consolas,monospace;white-space:pre-wrap;word-break:break-all}
.actions{margin-top:24px}
.btn{display:inline-block;padding:6px 18px;border-radius:6px;background:var(--accent);color:#fff;text-decoration:none;font-weight:500}
.btn:hover{filter:brightness(1.08)}
button.btn{font:inherit;cursor:pointer;border:0}
.btn.secondary{background:transparent;color:var(--text-1);border:1px solid var(--border)}
</style></head>
<body><main class="card {{.Tone}}">
<div class="brand"><i></i>明点服务器管理系统 · 控制台</div>
<div class="icon">{{if eq .Tone "loading"}}<div class="spin"></div>{{else if eq .Tone "java"}}<div class="spin i-wait"></div><svg class="i-ok" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M8 12.5l2.7 2.7L16 9.8"/></svg><svg class="i-fail" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 7.5v5.5"/><path d="M12 16.5h.01"/></svg>{{else if eq .Tone "warning"}}<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l7 3v5c0 4.5-3 8.2-7 9.5-4-1.3-7-5-7-9.5V6z"/><path d="M12 9v4"/><path d="M12 16h.01"/></svg>{{else}}<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 7.5v5.5"/><path d="M12 16.5h.01"/></svg>{{end}}</div>
<h1 id="title">{{.Title}}</h1>
<p>{{.Message}}</p>
{{if eq .Tone "java"}}<div id="status">正在准备…</div>{{end}}
{{with .Target}}<div class="target">{{.}}</div>{{end}}
{{with .Hints}}<ul>{{range .}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{with .Detail}}<details><summary>技术详情</summary><pre>{{.}}</pre></details>{{end}}
{{with .Retry}}<div class="actions"><a class="btn" href="{{.}}">重新连接</a></div>{{end}}
{{if eq .Tone "java"}}<div class="actions"><button class="btn secondary" onclick="history.back()">返回 BMC 管理页面</button></div>{{end}}
</main>
{{if eq .Tone "loading"}}<script>
// Ask the window to open the BMC whenever this page is shown, also when
// the user comes back to it through the history.
addEventListener('pageshow', function () { try { chrome.webview.postMessage('` + openMessage + `'); } catch (e) {} });
</script>{{end}}
{{if eq .Tone "java"}}<script>
(function () {
  var card = document.querySelector('.card'), title = document.getElementById('title'), status = document.getElementById('status');
  function show(cls, head, text) { card.className = 'card java ' + cls; title.textContent = head; status.textContent = text; }
  function tick() {
    fetch('` + javaStatusPath + `', { cache: 'no-store' }).then(function (r) { return r.json(); }).then(function (s) {
      if (s.state === 'running') { show('ok', 'Java 远程控制台已打开', '远程控制台在单独的 Java 窗口中运行。'); setTimeout(tick, 2000); }
      else if (s.state === 'failed') show('fail', 'Java 远程控制台启动失败', s.detail);
      else if (s.state === 'exited') s.detail === '已关闭' ? show('ok', 'Java 远程控制台已关闭', '远程控制台窗口已关闭。') : show('fail', 'Java 远程控制台已退出', s.detail);
      else { status.textContent = s.detail || '正在准备…'; setTimeout(tick, 600); }
    }).catch(function () { setTimeout(tick, 1500); });
  }
  tick();
})();
</script>{{end}}
</body></html>
`))

func (p page) render() string {
	var b bytes.Buffer
	if err := pageTemplate.Execute(&b, p); err != nil {
		return "<!doctype html><title>Error</title><p>" + template.HTMLEscapeString(p.Title) + "</p>"
	}
	return b.String()
}
