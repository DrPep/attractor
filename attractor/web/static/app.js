// Attractor web UI — single-file vanilla JS.
// Fetches /api/runs, renders a DOT graph with d3-graphviz, subscribes to SSE,
// and maps events onto node/edge styling plus a side panel.

const state = {
  runId: null,
  eventSource: null,
  nextSeq: 0,
  selectedNodeId: null,
  nodeStatus: {},         // node_id -> status string
  recentEvents: [],       // most recent events (all)
};

const els = {
  picker: document.getElementById("run-picker"),
  status: document.getElementById("run-status"),
  graph: document.getElementById("graph"),
  empty: document.getElementById("empty-msg"),
  title: document.getElementById("detail-title"),
  meta: document.getElementById("detail-meta"),
  prompt: document.getElementById("detail-prompt"),
  response: document.getElementById("detail-response"),
  statusDoc: document.getElementById("detail-status"),
  activity: document.getElementById("detail-activity"),
  events: document.getElementById("detail-events"),
};

// Agent activity grouped by node_id, in arrival order.
// Each entry: { kind, ts, payload, type? }
const agentActivityByNode = {};

// ── bootstrapping ────────────────────────────────────────────────────────

async function boot() {
  await refreshRunList();
  els.picker.addEventListener("change", () => selectRun(els.picker.value));

  // Priority: ?run=<id> in URL → first live run → first option.
  const params = new URLSearchParams(window.location.search);
  const want = params.get("run");
  const options = [...els.picker.options].filter(o => !o.disabled);
  let initial = null;
  if (want && options.find(o => o.value === want)) {
    initial = want;
  } else {
    const live = options.find(o => o.dataset.live === "true");
    initial = live ? live.value : (options[0] ? options[0].value : null);
  }
  if (initial) {
    els.picker.value = initial;
    selectRun(initial);
  }
  setInterval(refreshRunList, 5000);
}

async function refreshRunList() {
  const r = await fetch("/api/runs");
  if (!r.ok) return;
  const { runs } = await r.json();
  const current = els.picker.value;
  els.picker.innerHTML = "";
  if (runs.length === 0) {
    const opt = document.createElement("option");
    opt.textContent = "(no runs yet)";
    opt.disabled = true;
    els.picker.appendChild(opt);
    return;
  }
  for (const run of runs) {
    const opt = document.createElement("option");
    opt.value = run.run_id;
    opt.textContent = run.run_id + (run.live ? "  ●" : "");
    opt.dataset.live = run.live ? "true" : "false";
    els.picker.appendChild(opt);
  }
  if (current && runs.find(r => r.run_id === current)) {
    els.picker.value = current;
  }
}

// ── run selection + SSE ──────────────────────────────────────────────────

async function selectRun(runId) {
  if (!runId || runId === state.runId) return;
  closeStream();
  state.runId = runId;
  state.nextSeq = 0;
  state.nodeStatus = {};
  state.recentEvents = [];
  for (const k of Object.keys(agentActivityByNode)) delete agentActivityByNode[k];
  clearDetail();

  const r = await fetch(`/api/runs/${encodeURIComponent(runId)}`);
  if (!r.ok) {
    els.empty.textContent = `Failed to load run ${runId}`;
    els.empty.style.display = "flex";
    return;
  }
  const info = await r.json();
  state.nextSeq = info.next_seq || 0;
  state.nodeStatus = {};
  for (const [nid, entry] of Object.entries(info.node_state || {})) {
    if (entry.status) state.nodeStatus[nid] = entry.status;
  }

  if (info.graph_dot) {
    els.empty.style.display = "none";
    renderGraph(info.graph_dot, () => {
      applyAllStatuses();
      attachNodeClickHandlers();
    });
  } else {
    els.empty.textContent = `No graph recorded for ${runId}`;
    els.empty.style.display = "flex";
  }

  updateRunPill(info);
  if (info.live || !info.finished) openStream(runId);
}

function openStream(runId) {
  const url = `/api/runs/${encodeURIComponent(runId)}/events?since=${state.nextSeq}`;
  const es = new EventSource(url);
  state.eventSource = es;

  const dispatch = (kind) => (msg) => handleEvent(kind, JSON.parse(msg.data));
  const kinds = ["node_start", "node_end", "edge", "retry", "agent_event", "run_end"];
  for (const k of kinds) es.addEventListener(k, dispatch(k));

  es.onerror = () => {
    // EventSource auto-reconnects; nothing to do unless we want to surface it.
  };
}

function closeStream() {
  if (state.eventSource) {
    state.eventSource.close();
    state.eventSource = null;
  }
}

// ── event handling ───────────────────────────────────────────────────────

function handleEvent(kind, event) {
  state.nextSeq = Math.max(state.nextSeq, (event.seq ?? 0) + 1);
  state.recentEvents.unshift({ kind, ...event });
  state.recentEvents = state.recentEvents.slice(0, 200);
  if (state.selectedNodeId) renderRecentEventsForNode(state.selectedNodeId);

  const d = event.data || {};
  if (kind === "node_start" && d.node_id) {
    setNodeStatus(d.node_id, "running");
  } else if (kind === "node_end" && d.node_id) {
    setNodeStatus(d.node_id, d.outcome || "success");
  } else if (kind === "edge") {
    markEdgeTraversed(d.source, d.target);
    if (d.target && !state.nodeStatus[d.target]) {
      setNodeStatus(d.target, "queued");
    }
  } else if (kind === "retry" && d.node_id) {
    setNodeStatus(d.node_id, "retrying");
  } else if (kind === "agent_event") {
    recordAgentEvent(event);
  } else if (kind === "run_end") {
    updateRunPill({ live: false, finished: true });
    closeStream();
  }
}

// ── agent activity stream ────────────────────────────────────────────────

function recordAgentEvent(event) {
  const d = event.data || {};
  const nodeId = d.node_id;
  if (!nodeId) return;
  const entry = {
    type: d.type,
    payload: d.payload || {},
    ts: event.ts || 0,
  };
  const list = agentActivityByNode[nodeId] || (agentActivityByNode[nodeId] = []);
  // Coalesce streaming text/thinking deltas into a single growing entry so the
  // panel doesn't explode into hundreds of rows during a long response.
  const last = list[list.length - 1];
  if (last && last.type === entry.type && (entry.type === "text_delta" || entry.type === "thinking")) {
    last.payload = {
      ...last.payload,
      text: (last.payload.text || "") + (entry.payload.text || ""),
    };
    last.ts = entry.ts;
  } else {
    list.push(entry);
    // Cap per-node to keep DOM small.
    if (list.length > 400) list.splice(0, list.length - 400);
  }
  if (state.selectedNodeId === nodeId) renderAgentActivity(nodeId);
}

function renderAgentActivity(nodeId) {
  const list = agentActivityByNode[nodeId] || [];
  els.activity.innerHTML = "";
  if (list.length === 0) {
    const empty = document.createElement("div");
    empty.className = "activity-empty";
    empty.textContent = "No agent activity yet.";
    els.activity.appendChild(empty);
    return;
  }
  for (const entry of list) {
    const row = renderActivityRow(entry);
    if (row) els.activity.appendChild(row);
  }
  // Autoscroll to bottom so users see live progress.
  els.activity.scrollTop = els.activity.scrollHeight;
}

function renderActivityRow(entry) {
  const { type, payload, ts } = entry;
  const when = ts ? new Date(ts * 1000).toLocaleTimeString() : "";
  const row = document.createElement("div");
  row.className = `activity-row activity-${type}`;

  const head = document.createElement("div");
  head.className = "activity-head";
  const label = document.createElement("span");
  label.className = "activity-label";
  label.textContent = activityLabel(type);
  const time = document.createElement("span");
  time.className = "activity-time";
  time.textContent = when;
  head.appendChild(label);
  head.appendChild(time);
  row.appendChild(head);

  const body = document.createElement("div");
  body.className = "activity-body";

  switch (type) {
    case "thinking":
      body.textContent = payload.text || "";
      break;
    case "text_delta":
      body.textContent = payload.text || "";
      break;
    case "tool_call_start": {
      const name = document.createElement("code");
      name.textContent = payload.tool_name || "(tool)";
      body.appendChild(name);
      if (payload.args && Object.keys(payload.args).length) {
        const pre = document.createElement("pre");
        pre.textContent = formatArgs(payload.args);
        body.appendChild(pre);
      }
      break;
    }
    case "tool_call_end": {
      const ok = payload.success ? "ok" : "failed";
      const ms = typeof payload.duration_ms === "number"
        ? `  ${payload.duration_ms.toFixed(0)}ms`
        : "";
      body.textContent = `${payload.tool_name || ""} ${ok}${ms}`;
      if (!payload.success) row.classList.add("activity-fail");
      break;
    }
    case "tool_error":
      body.textContent = `${payload.tool_name || ""}: ${payload.error || ""}`;
      row.classList.add("activity-fail");
      break;
    case "llm_request":
      body.textContent = `${payload.model || ""}  (${payload.message_count} msgs)`;
      break;
    case "llm_response":
      body.textContent = `${payload.model || ""}  finish=${payload.finish_reason || "?"}  tokens=${payload.tokens ?? "?"}`;
      break;
    case "turn_start":
      body.textContent = `turn ${payload.turn_number}`;
      break;
    case "turn_end":
      body.textContent = `turn ${payload.turn_number} done (${payload.tool_calls} tool calls)`;
      break;
    case "steering_injected":
      body.textContent = payload.text || "";
      break;
    case "loop_detected":
      body.textContent = JSON.stringify(payload);
      row.classList.add("activity-warn");
      break;
    case "error":
      body.textContent = `${payload.error_type || "error"}: ${payload.error || ""}`;
      row.classList.add("activity-fail");
      break;
    default:
      // Skip low-signal events rather than cluttering the panel.
      return null;
  }

  row.appendChild(body);
  return row;
}

function activityLabel(type) {
  const map = {
    thinking: "thinking",
    text_delta: "assistant",
    tool_call_start: "tool →",
    tool_call_end: "tool ←",
    tool_error: "tool error",
    llm_request: "llm request",
    llm_response: "llm response",
    turn_start: "turn start",
    turn_end: "turn end",
    steering_injected: "steering",
    loop_detected: "loop detected",
    error: "error",
  };
  return map[type] || type;
}

function formatArgs(args) {
  try {
    const s = JSON.stringify(args, null, 2);
    return s.length > 2000 ? s.slice(0, 2000) + "\n… (truncated)" : s;
  } catch {
    return String(args);
  }
}

function setNodeStatus(nodeId, status) {
  state.nodeStatus[nodeId] = status;
  applyStatus(nodeId, status);
}

function applyAllStatuses() {
  for (const [nid, status] of Object.entries(state.nodeStatus)) {
    applyStatus(nid, status);
  }
}

function applyStatus(nodeId, status) {
  const el = findNodeEl(nodeId);
  if (el) el.setAttribute("data-status", status);
}

function markEdgeTraversed(source, target) {
  if (!source || !target) return;
  const edges = document.querySelectorAll("#graph .edge");
  for (const e of edges) {
    const title = e.querySelector("title");
    if (!title) continue;
    const t = title.textContent || "";
    if (t === `${source}->${target}` || t === `${source} -> ${target}`) {
      e.classList.add("traversed");
    }
  }
}

// ── graph rendering ──────────────────────────────────────────────────────

function renderGraph(dot, onReady) {
  els.graph.innerHTML = "";
  d3.select("#graph")
    .graphviz({ useWorker: false, fit: true, zoom: true })
    .renderDot(dot)
    .on("end", () => {
      onReady && onReady();
    });
}

function findNodeEl(nodeId) {
  const nodes = document.querySelectorAll("#graph .node");
  for (const n of nodes) {
    const title = n.querySelector("title");
    if (title && title.textContent === nodeId) return n;
  }
  return null;
}

function attachNodeClickHandlers() {
  const nodes = document.querySelectorAll("#graph .node");
  for (const n of nodes) {
    const title = n.querySelector("title");
    if (!title) continue;
    const nid = title.textContent;
    n.addEventListener("click", () => selectNode(nid));
  }
}

// ── detail panel ─────────────────────────────────────────────────────────

async function selectNode(nodeId) {
  state.selectedNodeId = nodeId;
  document.querySelectorAll("#graph .node.selected").forEach(n => n.classList.remove("selected"));
  const el = findNodeEl(nodeId);
  if (el) el.classList.add("selected");

  els.title.textContent = nodeId;
  els.meta.innerHTML = "";
  addMeta("status", state.nodeStatus[nodeId] || "—");

  els.prompt.textContent = "(loading…)";
  els.response.textContent = "";
  els.statusDoc.textContent = "";

  try {
    const r = await fetch(`/api/runs/${encodeURIComponent(state.runId)}/nodes/${encodeURIComponent(nodeId)}`);
    if (r.ok) {
      const data = await r.json();
      els.prompt.textContent = data.prompt || "(no prompt)";
      els.response.textContent = data.response || "(no response yet)";
      els.statusDoc.textContent = Object.keys(data.status || {}).length
        ? JSON.stringify(data.status, null, 2)
        : "(no status yet)";
    } else {
      els.prompt.textContent = "(no artifacts on disk yet)";
      els.response.textContent = "";
      els.statusDoc.textContent = "";
    }
  } catch (e) {
    els.prompt.textContent = `error: ${e.message}`;
  }

  renderAgentActivity(nodeId);
  renderRecentEventsForNode(nodeId);
}

function addMeta(key, value) {
  const dt = document.createElement("dt"); dt.textContent = key;
  const dd = document.createElement("dd"); dd.textContent = value;
  els.meta.appendChild(dt); els.meta.appendChild(dd);
}

function renderRecentEventsForNode(nodeId) {
  els.events.innerHTML = "";
  const relevant = state.recentEvents.filter(e => (e.data || {}).node_id === nodeId).slice(0, 40);
  for (const ev of relevant) {
    const li = document.createElement("li");
    const when = new Date((ev.ts || 0) * 1000).toLocaleTimeString();
    const payload = JSON.stringify(ev.data).slice(0, 140);
    li.textContent = `${when}  ${ev.kind}  ${payload}`;
    els.events.appendChild(li);
  }
}

function clearDetail() {
  state.selectedNodeId = null;
  els.title.textContent = "Node";
  els.meta.innerHTML = "";
  els.prompt.textContent = "";
  els.response.textContent = "";
  els.statusDoc.textContent = "";
  els.activity.innerHTML = '<div class="activity-empty">No agent activity yet.</div>';
  els.events.innerHTML = "";
}

function updateRunPill(info) {
  els.status.classList.remove("live", "done");
  if (info.live) {
    els.status.textContent = "live";
    els.status.classList.add("live");
  } else if (info.finished) {
    els.status.textContent = "done";
    els.status.classList.add("done");
  } else {
    els.status.textContent = "";
  }
}

boot();
