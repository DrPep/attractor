// Attractor web UI — single-file vanilla JS.
// Fetches /api/runs, renders the pipeline DOT with d3-graphviz, subscribes to
// the SSE event stream, and maps live events onto node colours + a side panel.

const STATUS_COLORS = {
  running: "#f7c948",
  success: "#3fb950",
  partial_success: "#58a6ff",
  fail: "#f85149",
  error: "#f85149",
  retrying: "#d29922",
  queued: "#9aa7b4",
};

const state = {
  runId: null,
  eventSource: null,
  nextSeq: 0,
  selectedNodeId: null,
  nodeStatus: {},      // node_id -> status string
  nodeTokens: {},      // node_id -> accumulated total tokens
  nodeReasoning: {},   // node_id -> concatenated reasoning text
  recentEvents: [],    // most recent events across the run
  runningNodeId: null, // node currently executing (for the status bar)
  graphReady: false,
  // Detail-pane fields for the selected node, loaded with its status.json.
  detailNotes: "",
  detailModel: "",
  detailProvider: "",
};

const els = {};
function $(id) { return document.getElementById(id); }

// ── bootstrapping ────────────────────────────────────────────────────────

async function boot() {
  for (const id of ["run-picker", "run-status", "graph", "empty-msg", "legend",
    "detail-title", "detail-badge", "detail-body", "detail-meta", "detail-prompt",
    "detail-response", "detail-status", "detail-events", "theme-toggle",
    "status-bar", "status-bar-node", "status-bar-text",
    "detail-reasoning", "detail-reasoning-wrap",
    "steer-box", "steer-input", "steer-send", "steer-status"]) {
    els[id] = $(id);
  }
  els["detail-pane"] = $("detail-title").closest("aside");

  initTheme();
  els["run-picker"].addEventListener("change", () => selectRun(els["run-picker"].value));
  els["steer-send"].addEventListener("click", sendSteer);
  els["steer-input"].addEventListener("keydown", (e) => {
    // ⌘/Ctrl+Enter sends; plain Enter keeps a newline.
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); sendSteer(); }
  });

  await refreshRunList();

  const params = new URLSearchParams(window.location.search);
  const want = params.get("run");
  const options = [...els["run-picker"].options].filter(o => !o.disabled);
  let initial = null;
  if (want && options.find(o => o.value === want)) {
    initial = want;
  } else {
    const live = options.find(o => o.dataset.live === "true");
    initial = live ? live.value : (options[0] ? options[0].value : null);
  }
  if (initial) {
    els["run-picker"].value = initial;
    selectRun(initial);
  }
  setInterval(refreshRunList, 5000);
}

function initTheme() {
  const saved = localStorage.getItem("attractor-theme");
  if (saved) document.documentElement.dataset.theme = saved;
  els["theme-toggle"].addEventListener("click", () => {
    const next = document.documentElement.dataset.theme === "light" ? "dark" : "light";
    document.documentElement.dataset.theme = next;
    localStorage.setItem("attractor-theme", next);
  });
}

async function refreshRunList() {
  let runs;
  try {
    const r = await fetch("/api/runs");
    if (!r.ok) return;
    ({ runs } = await r.json());
  } catch { return; }

  const picker = els["run-picker"];
  const current = picker.value;
  picker.innerHTML = "";
  if (!runs || runs.length === 0) {
    const opt = document.createElement("option");
    opt.textContent = "(no runs yet)";
    opt.disabled = true;
    picker.appendChild(opt);
    return;
  }
  for (const run of runs) {
    const opt = document.createElement("option");
    opt.value = run.run_id;
    opt.textContent = run.run_id + (run.live ? "  ●" : "");
    opt.dataset.live = run.live ? "true" : "false";
    picker.appendChild(opt);
  }
  if (current && runs.find(r => r.run_id === current)) picker.value = current;
}

// ── run selection + SSE ──────────────────────────────────────────────────

async function selectRun(runId) {
  if (!runId || runId === state.runId) return;
  closeStream();
  Object.assign(state, {
    runId, nextSeq: 0, selectedNodeId: null,
    nodeStatus: {}, nodeTokens: {}, nodeReasoning: {},
    recentEvents: [], runningNodeId: null, graphReady: false,
  });
  clearReasoningBar();
  clearDetail();

  let info;
  try {
    const r = await fetch(`/api/runs/${encodeURIComponent(runId)}`);
    if (!r.ok) throw new Error();
    info = await r.json();
  } catch {
    showEmpty(`Failed to load run ${runId}`);
    return;
  }

  state.nextSeq = info.next_seq || 0;
  for (const [nid, entry] of Object.entries(info.node_state || {})) {
    if (entry.status) state.nodeStatus[nid] = entry.status;
    if (entry.tokens) state.nodeTokens[nid] = entry.tokens;
  }

  if (info.graph_dot) {
    els["empty-msg"].style.display = "none";
    els["legend"].hidden = false;
    renderGraph(info.graph_dot, () => {
      state.graphReady = true;
      applyAllStatuses();
      applyAllTokens();
      attachNodeClickHandlers();
    });
  } else {
    showEmpty(`No graph recorded for ${runId}`);
  }

  updateRunPill(info);
  if (info.live || !info.finished) openStream(runId);
}

function showEmpty(msg) {
  els["empty-msg"].textContent = msg;
  els["empty-msg"].style.display = "flex";
  els["legend"].hidden = true;
}

function openStream(runId) {
  const url = `/api/runs/${encodeURIComponent(runId)}/events?since=${state.nextSeq}`;
  const es = new EventSource(url);
  state.eventSource = es;
  const dispatch = (kind) => (msg) => handleEvent(kind, JSON.parse(msg.data));
  for (const k of ["node_start", "node_end", "edge", "retry", "agent_event", "run_end"]) {
    es.addEventListener(k, dispatch(k));
  }
  es.onerror = () => {};  // EventSource auto-reconnects
}

function closeStream() {
  if (state.eventSource) { state.eventSource.close(); state.eventSource = null; }
}

// ── event handling ───────────────────────────────────────────────────────

function handleEvent(kind, event) {
  state.nextSeq = Math.max(state.nextSeq, (event.seq ?? 0) + 1);
  state.recentEvents.unshift({ kind, ...event });
  state.recentEvents = state.recentEvents.slice(0, 200);
  if (state.selectedNodeId) renderRecentEventsForNode(state.selectedNodeId);

  const d = event.data || {};
  switch (kind) {
    case "node_start":
      if (d.node_id) {
        setNodeStatus(d.node_id, "running");
        state.runningNodeId = d.node_id;
      }
      break;
    case "node_end":
      if (d.node_id) {
        setNodeStatus(d.node_id, d.outcome || "success");
        if (d.node_id === state.runningNodeId) clearReasoningBar();
        if (d.node_id === state.selectedNodeId) {
          loadNodeDetail(d.node_id);
          updateSteerBox(d.node_id);
        }
      }
      break;
    case "edge":
      if (d.target && !state.nodeStatus[d.target]) setNodeStatus(d.target, "queued");
      break;
    case "retry":
      if (d.node_id) setNodeStatus(d.node_id, "retrying");
      break;
    case "agent_event":
      if (!d.node_id) break;
      if (d.type === "llm_response") {
        const tok = (d.payload && d.payload.tokens) || 0;
        if (tok) addNodeTokens(d.node_id, tok);
      } else if (d.type === "thinking") {
        const text = (d.payload && d.payload.text) || "";
        if (text) addNodeReasoning(d.node_id, text);
      }
      break;
    case "steer":
      // A steering message the UI (or someone) injected; surface it in the bar.
      if (d.node_id) showReasoningBar(d.node_id, "↩ feedback sent: " + (d.message || ""));
      break;
    case "run_end":
      updateRunPill({ live: false, finished: true });
      closeStream();
      break;
  }
}

// ── graph rendering ──────────────────────────────────────────────────────

let graphviz = null;

function renderGraph(dot, onDone) {
  els["graph"].innerHTML = "";
  graphviz = d3.select("#graph").graphviz({ useWorker: false, fit: true, zoom: true })
    .transition(() => d3.transition().duration(250));
  graphviz.renderDot(styleDot(dot)).on("end", onDone);
}

// Inject rendering defaults: a transparent canvas (so the pane shows through)
// and a sans-serif font that Graphviz also uses to size the nodes. Injected
// defaults come first, so anything the pipeline DOT sets still wins.
function styleDot(dot) {
  const i = dot.indexOf("{");
  if (i < 0) return dot;
  const inject =
    '\n  graph [bgcolor="transparent"];' +
    '\n  node [fontname="Helvetica"];' +
    '\n  edge [fontname="Helvetica"];\n';
  return dot.slice(0, i + 1) + inject + dot.slice(i + 1);
}

function nodeSelection(nodeId) {
  // d3-graphviz emits each node as <g class="node"> with a <title> = node id.
  return d3.select("#graph").selectAll("g.node")
    .filter(function () {
      const t = d3.select(this).select("title").text();
      return t === nodeId;
    });
}

function setNodeStatus(nodeId, status) {
  state.nodeStatus[nodeId] = status;
  if (state.graphReady) colorizeNode(nodeId, status);
}

function applyAllStatuses() {
  for (const [nid, status] of Object.entries(state.nodeStatus)) colorizeNode(nid, status);
}

function colorizeNode(nodeId, status) {
  const color = STATUS_COLORS[status];
  if (!color) return;
  const sel = nodeSelection(nodeId);
  sel.selectAll("ellipse, polygon, path")
    .style("stroke", color)
    .style("stroke-width", 2.5);
  sel.selectAll("ellipse, polygon")
    .style("fill", fade(color));
}

// ── per-node token usage ─────────────────────────────────────────────────

function addNodeTokens(nodeId, tokens) {
  state.nodeTokens[nodeId] = (state.nodeTokens[nodeId] || 0) + tokens;
  if (state.graphReady) renderNodeTokens(nodeId);
  if (nodeId === state.selectedNodeId) renderDetailMeta(nodeId);
}

function applyAllTokens() {
  for (const nid of Object.keys(state.nodeTokens)) renderNodeTokens(nid);
}

function formatTokens(n) {
  if (n >= 1000) return (n / 1000).toFixed(n >= 10000 ? 0 : 1) + "k";
  return String(n);
}

// Draw (or update) a small "N tok" badge just below a node's shape. The badge
// is re-created after each graph render, so it's driven off state.nodeTokens.
function renderNodeTokens(nodeId) {
  const tokens = state.nodeTokens[nodeId];
  if (!tokens) return;
  const sel = nodeSelection(nodeId);
  if (sel.empty()) return;
  const shape = sel.selectAll("ellipse, polygon, path").node();
  if (!shape) return;
  const b = shape.getBBox();
  let badge = sel.select("text.token-badge");
  if (badge.empty()) {
    badge = sel.append("text").attr("class", "token-badge").attr("text-anchor", "middle");
  }
  badge
    .attr("x", b.x + b.width / 2)
    .attr("y", b.y + b.height + 13)
    .text(formatTokens(tokens) + " tok");
}

// ── reasoning (model thinking) ───────────────────────────────────────────

function addNodeReasoning(nodeId, text) {
  const prev = state.nodeReasoning[nodeId] || "";
  state.nodeReasoning[nodeId] = prev ? prev + "\n\n" + text : text;
  // The status bar tracks whichever node is currently running.
  if (nodeId === state.runningNodeId) showReasoningBar(nodeId, lastLine(text));
  if (nodeId === state.selectedNodeId) renderReasoning(nodeId);
}

// lastLine returns the final non-empty line of a reasoning chunk — the freshest
// thought — for the single-line status bar.
function lastLine(text) {
  const lines = text.split("\n").map(s => s.trim()).filter(Boolean);
  return lines.length ? lines[lines.length - 1] : text.trim();
}

function showReasoningBar(nodeId, text) {
  const bar = els["status-bar"];
  if (!bar) return;
  bar.hidden = false;
  els["status-bar-node"].textContent = nodeId;
  els["status-bar-text"].textContent = text;
}

function clearReasoningBar() {
  const bar = els["status-bar"];
  if (!bar) return;
  bar.hidden = true;
  els["status-bar-node"].textContent = "";
  els["status-bar-text"].textContent = "";
}

function renderReasoning(nodeId) {
  const pre = els["detail-reasoning"];
  const wrap = els["detail-reasoning-wrap"];
  if (!pre) return;
  const text = state.nodeReasoning[nodeId] || "";
  pre.textContent = text;
  if (wrap) wrap.hidden = !text;
}

// translucent fill derived from a hex stroke color
function fade(hex) {
  const n = parseInt(hex.slice(1), 16);
  const r = (n >> 16) & 255, g = (n >> 8) & 255, b = n & 255;
  return `rgba(${r},${g},${b},0.16)`;
}

function attachNodeClickHandlers() {
  d3.select("#graph").selectAll("g.node").on("click", function () {
    const nodeId = d3.select(this).select("title").text();
    selectNode(nodeId);
  });
}

// ── detail pane ──────────────────────────────────────────────────────────

function selectNode(nodeId) {
  state.selectedNodeId = nodeId;
  d3.select("#graph").selectAll("g.node").classed("selected", false);
  nodeSelection(nodeId).classed("selected", true);
  els["detail-pane"].classList.remove("empty");
  els["detail-title"].textContent = nodeId;
  els["detail-body"].hidden = false;
  loadNodeDetail(nodeId);
  renderReasoning(nodeId);
  renderRecentEventsForNode(nodeId);
  updateSteerBox(nodeId);
}

async function loadNodeDetail(nodeId) {
  const status = state.nodeStatus[nodeId] || "";
  const badge = els["detail-badge"];
  badge.textContent = status;
  badge.style.color = STATUS_COLORS[status] || "";

  let data = null;
  try {
    const r = await fetch(`/api/runs/${encodeURIComponent(state.runId)}/nodes/${encodeURIComponent(nodeId)}`);
    if (r.ok) data = await r.json();
  } catch {}

  els["detail-prompt"].textContent = data?.prompt || "";
  els["detail-response"].textContent = data?.response || "";
  els["detail-status"].textContent = data?.status ? JSON.stringify(data.status, null, 2) : "";

  state.detailNotes = data?.status?.notes || "";
  // Only nodes that ran an LLM record these; they land with status.json when the
  // node finishes, so a node still running shows no model yet.
  state.detailModel = data?.status?.model || "";
  state.detailProvider = data?.status?.provider || "";
  renderDetailMeta(nodeId);
}

function renderDetailMeta(nodeId) {
  const meta = els["detail-meta"];
  meta.innerHTML = "";
  const status = state.nodeStatus[nodeId] || "";
  const rows = [["node", nodeId], ["status", status || "—"]];
  if (state.detailModel) {
    const provider = state.detailProvider;
    rows.push(["model", provider ? `${state.detailModel} (${provider})` : state.detailModel]);
  }
  const tokens = state.nodeTokens[nodeId];
  if (tokens) rows.push(["tokens", tokens.toLocaleString() + " total"]);
  if (state.detailNotes) rows.push(["notes", state.detailNotes]);
  for (const [k, v] of rows) {
    const dt = document.createElement("dt"); dt.textContent = k;
    const dd = document.createElement("dd"); dd.textContent = v;
    meta.append(dt, dd);
  }
}

function renderRecentEventsForNode(nodeId) {
  const list = els["detail-events"];
  list.innerHTML = "";
  const evs = state.recentEvents.filter(e => (e.data || {}).node_id === nodeId).slice(0, 30);
  for (const e of evs) {
    const li = document.createElement("li");
    const k = document.createElement("span");
    k.className = "k"; k.textContent = e.kind;
    li.append(k, document.createTextNode(" " + summarize(e)));
    list.appendChild(li);
  }
}

function summarize(e) {
  const d = e.data || {};
  if (e.kind === "node_end") return d.outcome || "";
  if (e.kind === "retry") return `attempt ${d.attempt}`;
  if (e.kind === "steer") return d.message || "";
  if (e.kind === "agent_event") {
    const p = d.payload || {};
    switch (d.type) {
      case "thinking": return truncate(lastLine(p.text || ""), 80);
      case "tool_call_start": return `${p.tool_name || "tool"}(…)`;
      case "tool_call_end": return `${p.tool_name || "tool"} ${p.success ? "ok" : "failed"}`;
      case "tool_error": return `${p.tool_name || "tool"}: ${p.error || ""}`;
      case "steering_injected": return `↩ ${truncate(p.text || "", 80)}`;
      case "llm_response": return p.tokens ? `${p.tokens} tok` : "response";
      default: return d.type || "";
    }
  }
  if (e.kind === "edge") return `→ ${d.target || ""}`;
  return "";
}

function truncate(s, n) { return s.length > n ? s.slice(0, n - 1) + "…" : s; }

// ── steering (inline corrective feedback) ────────────────────────────────

// Show the steer box only while the node is actively running.
function updateSteerBox(nodeId) {
  const box = els["steer-box"];
  if (!box) return;
  const running = state.nodeStatus[nodeId] === "running";
  box.hidden = !running;
  if (!running) els["steer-status"].textContent = "";
}

async function sendSteer() {
  const nodeId = state.selectedNodeId;
  const input = els["steer-input"];
  const msg = input.value.trim();
  if (!nodeId || !msg) return;
  els["steer-status"].textContent = "sending…";
  try {
    const r = await fetch(
      `/api/runs/${encodeURIComponent(state.runId)}/nodes/${encodeURIComponent(nodeId)}/steer`,
      { method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message: msg }) });
    if (r.status === 202) {
      input.value = "";
      els["steer-status"].textContent = "queued — applies before the next model turn";
    } else if (r.status === 404 || r.status === 409) {
      els["steer-status"].textContent = "node is no longer accepting feedback";
    } else {
      els["steer-status"].textContent = "failed to send";
    }
  } catch {
    els["steer-status"].textContent = "failed to send";
  }
}

function clearDetail() {
  state.selectedNodeId = null;
  els["detail-pane"].classList.add("empty");
  els["detail-body"].hidden = true;
  els["detail-title"].textContent = "Node";
  els["detail-badge"].textContent = "";
  if (els["steer-box"]) els["steer-box"].hidden = true;
}

// ── run status pill ──────────────────────────────────────────────────────

function updateRunPill(info) {
  const pill = els["run-status"];
  if (info.live) {
    pill.textContent = "live";
    pill.className = "pill live";
  } else {
    pill.textContent = "finished";
    pill.className = "pill done";
  }
}

boot();
