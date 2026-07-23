// SPDX-License-Identifier: Apache-2.0

package tunnelhub

import "net/http"

// NewAdmin returns the handler for the hub's admin overview page. The page is
// self-contained (inline CSS/JS, no external assets) and refreshes itself from
// GET /api/backends.
func NewAdmin(_ *Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(adminHTML))
	})
}

const adminHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>mxcli tunnel-hub — previews</title>
<style>
  :root { color-scheme: light dark; --bg:#fff; --fg:#1a1a2e; --mut:#6b7280; --line:#e5e7eb; --row:#f9fafb; --accent:#3b3bd6; }
  @media (prefers-color-scheme: dark){ :root{ --bg:#0f1020; --fg:#e6e6f0; --mut:#9aa0b4; --line:#26283b; --row:#171a2e; --accent:#8f8fff; } }
  * { box-sizing:border-box; }
  body { font:15px/1.45 system-ui,sans-serif; margin:0; background:var(--bg); color:var(--fg); }
  header { padding:1.1rem 1.4rem; border-bottom:1px solid var(--line); display:flex; align-items:baseline; gap:.8rem; flex-wrap:wrap; }
  h1 { font-size:1.05rem; margin:0; font-weight:650; }
  .meta { color:var(--mut); font-size:.85rem; }
  .wrap { padding:1rem 1.4rem; overflow-x:auto; }
  table { border-collapse:collapse; width:100%; min-width:52rem; }
  th,td { text-align:left; padding:.5rem .7rem; border-bottom:1px solid var(--line); white-space:nowrap; }
  th { font-size:.72rem; text-transform:uppercase; letter-spacing:.04em; color:var(--mut); cursor:pointer; user-select:none; }
  th.sorted::after { content:" \25BC"; font-size:.7em; }
  tbody tr:nth-child(even){ background:var(--row); }
  td.url a { color:var(--accent); text-decoration:none; }
  td.url a:hover { text-decoration:underline; }
  .dot { display:inline-block; width:.62rem; height:.62rem; border-radius:50%; margin-right:.4rem; vertical-align:-1px; }
  .available .dot { background:#22c55e; } .stale .dot { background:#f59e0b; }
  .available { color:inherit; } .stale { color:var(--mut); }
  .sol { color:var(--mut); font-size:.82rem; }
  .empty { color:var(--mut); padding:2rem 0; }
  code { font-family:ui-monospace,monospace; }
</style></head>
<body>
<header>
  <h1>mxcli tunnel-hub</h1>
  <span class="meta" id="count">…</span>
  <span class="meta" style="margin-left:auto" id="updated"></span>
</header>
<div class="wrap">
  <table>
    <thead><tr>
      <th data-k="availability">Status</th>
      <th data-k="solution">Solution</th>
      <th data-k="project">Project</th>
      <th data-k="branch">Branch</th>
      <th data-k="url">URL</th>
      <th data-k="registeredAt">Registered</th>
      <th data-k="lastSeenAt">Last seen</th>
      <th data-k="lastUsedAt">Last used</th>
      <th data-k="uptimeSec">Uptime</th>
    </tr></thead>
    <tbody id="rows"></tbody>
  </table>
  <div class="empty" id="empty" hidden>No previews registered yet. Start one with <code>mxcli run --hub …</code></div>
</div>
<script>
(function(){
  var sortKey = "lastUsedAt", data = [];
  function ago(t){
    if(!t || t.startsWith("0001")) return "—";
    var s = Math.max(0,(Date.now()-new Date(t))/1000);
    if(s<60) return Math.floor(s)+"s ago";
    if(s<3600) return Math.floor(s/60)+"m ago";
    if(s<86400) return Math.floor(s/3600)+"h ago";
    return Math.floor(s/86400)+"d ago";
  }
  function dur(sec){
    if(sec<60) return sec+"s"; if(sec<3600) return Math.floor(sec/60)+"m";
    if(sec<86400) return Math.floor(sec/3600)+"h"; return Math.floor(sec/86400)+"d";
  }
  function cmp(a,b){
    if(sortKey==="project"){
      return (a.solution||"").localeCompare(b.solution||"") ||
             (a.project||"").localeCompare(b.project||"") ||
             (a.branch||"").localeCompare(b.branch||"");
    }
    if(sortKey==="availability") return (a.availability>b.availability?1:-1);
    if(sortKey==="uptimeSec") return b.uptimeSec-a.uptimeSec;
    // time keys: newest first
    return new Date(b[sortKey]||0)-new Date(a[sortKey]||0);
  }
  function esc(s){ var d=document.createElement("div"); d.textContent=s==null?"":s; return d.innerHTML; }
  function render(){
    data.sort(cmp);
    document.querySelectorAll("th").forEach(function(th){ th.classList.toggle("sorted", th.dataset.k===sortKey); });
    var rows = data.map(function(b){
      var name = (b.prefix?esc(b.prefix)+" · ":"")+esc(b.project);
      return "<tr class='"+esc(b.availability)+"'>"+
        "<td><span class='dot'></span>"+esc(b.availability)+"</td>"+
        "<td class='sol'>"+esc(b.solution||"—")+"</td>"+
        "<td>"+name+"</td>"+
        "<td><code>"+esc(b.branch||"—")+"</code></td>"+
        "<td class='url'><a href='"+esc(b.url)+"' target='_blank' rel='noopener'>"+esc(b.url.replace(/^https?:\/\//,""))+"</a></td>"+
        "<td>"+ago(b.registeredAt)+"</td>"+
        "<td>"+ago(b.lastSeenAt)+"</td>"+
        "<td>"+ago(b.lastUsedAt)+"</td>"+
        "<td>"+dur(b.uptimeSec)+"</td></tr>";
    }).join("");
    document.getElementById("rows").innerHTML = rows;
    document.getElementById("empty").hidden = data.length>0;
    document.getElementById("count").textContent = data.length+" preview"+(data.length===1?"":"s");
    document.getElementById("updated").textContent = "updated "+new Date().toLocaleTimeString();
  }
  function load(){
    fetch("/api/backends").then(function(r){return r.json();}).then(function(j){ data=j||[]; render(); }).catch(function(){});
  }
  document.querySelectorAll("th").forEach(function(th){
    th.addEventListener("click", function(){ sortKey=th.dataset.k; render(); });
  });
  load(); setInterval(load, 5000);
})();
</script>
</body></html>`
