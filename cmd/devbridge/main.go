// Command devbridge serves the built frontend to an ordinary browser and
// forwards the Wails binding calls to the real Go services over HTTP. It is
// a development and end-to-end testing tool for machines without WebView2
// (Linux, macOS, CI); the desktop application does not use it.
//
//	cd frontend && npm run build && cd ..
//	go run ./cmd/devbridge -sim 3
//	# open http://127.0.0.1:34115
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/12312bg/mdbmc/internal/app"
	"github.com/12312bg/mdbmc/internal/bmcsim"
	"github.com/12312bg/mdbmc/internal/console"
)

type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func (h *hub) emit(name string, data any) {
	b, err := json.Marshal(map[string]any{"name": name, "data": data})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.subs {
		select {
		case c <- b:
		default: // slow subscriber; drop
		}
	}
}

func (h *hub) serve(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	c := make(chan []byte, 256)
	h.mu.Lock()
	h.subs[c] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.subs, c)
		h.mu.Unlock()
	}()
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-c:
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
	}
}

type caller struct {
	services map[string]reflect.Value
}

func (c *caller) methods() map[string][]string {
	out := map[string][]string{}
	for name, v := range c.services {
		t := v.Type()
		for i := 0; i < t.NumMethod(); i++ {
			out[name] = append(out[name], t.Method(i).Name)
		}
		sort.Strings(out[name])
	}
	return out
}

func (c *caller) call(w http.ResponseWriter, r *http.Request) {
	// A custom header forces a CORS preflight, which this server never
	// answers, so other web pages cannot make calls (CSRF).
	if r.Header.Get("X-Mdbmc-Bridge") != "1" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Service string            `json:"service"`
		Method  string            `json:"method"`
		Args    []json.RawMessage `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	svc, ok := c.services[req.Service]
	if !ok {
		http.Error(w, "unknown service", http.StatusNotFound)
		return
	}
	m := svc.MethodByName(req.Method)
	if !m.IsValid() || m.Type().NumIn() != len(req.Args) {
		http.Error(w, "unknown method or wrong argument count", http.StatusNotFound)
		return
	}
	in := make([]reflect.Value, len(req.Args))
	for i, raw := range req.Args {
		p := reflect.New(m.Type().In(i))
		if err := json.Unmarshal(raw, p.Interface()); err != nil {
			http.Error(w, fmt.Sprintf("arg %d: %v", i, err), http.StatusBadRequest)
			return
		}
		in[i] = p.Elem()
	}
	out := m.Call(in)
	resp := map[string]any{"result": nil}
	errType := reflect.TypeOf((*error)(nil)).Elem()
	for _, o := range out {
		if o.Type().Implements(errType) {
			if !o.IsNil() {
				resp = map[string]any{"error": o.Interface().(error).Error()}
			}
			continue
		}
		resp["result"] = o.Interface()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

const bridgeJS = `(function(){
  var methods = %s;
  window.go = { app: {} };
  Object.keys(methods).forEach(function (svc) {
    window.go.app[svc] = {};
    methods[svc].forEach(function (m) {
      window.go.app[svc][m] = function () {
        var args = Array.prototype.slice.call(arguments);
        return fetch('/call', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Mdbmc-Bridge': '1' },
          body: JSON.stringify({ service: svc, method: m, args: args }) })
          .then(function (r) { return r.ok ? r.json() : r.text().then(function (t) { throw t; }); })
          .then(function (j) { if (j.error !== undefined) throw j.error; return j.result; });
      };
    });
  });
  var listeners = {};
  window.runtime = {
    EventsOnMultiple: function (name, cb, max) {
      var l = { cb: cb, left: max };
      (listeners[name] = listeners[name] || []).push(l);
      return function () { listeners[name] = (listeners[name] || []).filter(function (x) { return x !== l; }); };
    },
    EventsOff: function (name) { delete listeners[name]; },
    EventsOffAll: function () { listeners = {}; },
    EventsEmit: function () {},
    BrowserOpenURL: function (u) { window.open(u, '_blank'); },
  };
  var es = new EventSource('/events');
  es.onmessage = function (e) {
    var msg = JSON.parse(e.data);
    (listeners[msg.name] || []).slice().forEach(function (l) {
      l.cb(msg.data);
      if (l.left > 0 && --l.left === 0) listeners[msg.name] = listeners[msg.name].filter(function (x) { return x !== l; });
    });
  };
})();`

// loopbackOnly rejects requests whose Host header is not the address we
// listen on, which defeats DNS-rebinding pages reading responses.
func loopbackOnly(addr string, next http.Handler) http.Handler {
	_, port, _ := net.SplitHostPort(addr)
	allowed := map[string]bool{addr: true, "127.0.0.1:" + port: true, "localhost:" + port: true, "[::1]:" + port: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[r.Host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	addr := flag.String("addr", "127.0.0.1:34115", "listen address (keep it on loopback)")
	dist := flag.String("dist", "frontend/dist", "built frontend directory")
	dataDir := flag.String("data", "", "data directory (default: a new temporary directory)")
	sims := flag.Int("sim", 0, "also start this many simulated BMCs")
	flag.Parse()

	dir := *dataDir
	if dir == "" {
		var err error
		if dir, err = os.MkdirTemp("", "mdbmc-devbridge-"); err != nil {
			log.Fatal(err)
		}
	}
	for i := 0; i < *sims; i++ {
		s, err := bmcsim.Start(bmcsim.Options{Index: i, IPMIPort: 6230 + i, HTTPSPort: 8443 + i})
		if err != nil {
			log.Fatalf("simulator %d: %v", i, err)
		}
		defer s.Close()
		log.Printf("simulated BMC #%d: 127.0.0.1 IPMI %d HTTPS %d (ADMIN/ADMIN)", i, s.IPMIPort, s.HTTPSPort)
	}

	h := &hub{subs: map[chan []byte]struct{}{}}
	a, err := app.New(dir, func(req console.Request) error {
		h.emit("devbridge:console", map[string]any{"title": req.Title, "url": req.Spec.URL, "autoLogin": req.Spec.AutoLogin})
		log.Printf("console window requested: %s %s (auto-login %v)", req.Title, req.Spec.URL, req.Spec.AutoLogin)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	a.Startup(context.Background(), h.emit)
	defer a.Shutdown()

	c := &caller{services: map[string]reflect.Value{}}
	for _, s := range a.Bindings() {
		c.services[reflect.TypeOf(s).Elem().Name()] = reflect.ValueOf(s)
	}
	methodsJSON, _ := json.Marshal(c.methods())

	index, err := os.ReadFile(filepath.Join(*dist, "index.html"))
	if err != nil {
		log.Fatalf("read %s (run npm run build first): %v", *dist, err)
	}
	page := strings.Replace(string(index), "<head>", `<head><script src="/bridge.js"></script>`, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /call", c.call)
	mux.HandleFunc("GET /events", h.serve)
	mux.HandleFunc("GET /bridge.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		fmt.Fprintf(w, bridgeJS, methodsJSON)
	})
	files := http.FileServer(http.Dir(*dist))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, page)
			return
		}
		files.ServeHTTP(w, r)
	})
	log.Printf("data dir %s", dir)
	log.Printf("open http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, loopbackOnly(*addr, mux)))
}
