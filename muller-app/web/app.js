"use strict";
// Saisie Ligne Muller — client. Entries are saved with PUT /api/entries/{id}
// (client-generated id, so retries are idempotent). When the network is down
// the write goes to a local outbox and is replayed when the server is back.

const CAUSES = ["Panne presse", "Panne transfert / robot", "Outillage (casse, réglage)", "Manque bobine / matière", "Qualité / retouche réglage", "Maintenance préventive", "Attente personnel", "Attente cariste / bacs", "Autre"];
const $ = (id) => document.getElementById(id);
const state = { poste: "Matin", equipe: "MR", equipeManual: false, stops: [], editId: null, week: [], viewWeek: null, confirmDel: null, online: true };

// ---------- helpers ----------
function isoWeek(d) { const t = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate())); const day = t.getUTCDay() || 7; t.setUTCDate(t.getUTCDate() + 4 - day); const y0 = new Date(Date.UTC(t.getUTCFullYear(), 0, 1)); return { year: t.getUTCFullYear(), week: Math.ceil(((t - y0) / 864e5 + 1) / 7) }; }
function parseDate(s) { const [y, m, d] = s.split("-").map(Number); return new Date(y, m - 1, d); }
function fmtDate(d) { return d.getFullYear() + "-" + String(d.getMonth() + 1).padStart(2, "0") + "-" + String(d.getDate()).padStart(2, "0"); }
function teamFor(week, poste) { if (poste === "Nuit") return "Nuit"; return (poste === "Matin") === (week % 2 === 1) ? "MR" : "BC"; }
function n(v) { const x = parseFloat(String(v).replace(",", ".")); return isFinite(x) ? x : 0; }
function fmtH(x) { return (Math.round(x * 100) / 100).toLocaleString("fr-FR"); }
function newId() { const b = new Uint8Array(12); crypto.getRandomValues(b); return Array.from(b, (x) => x.toString(16).padStart(2, "0")).join(""); }
function toast(msg) { const t = $("toast"); t.textContent = msg; t.hidden = false; clearTimeout(toast._t); toast._t = setTimeout(() => (t.hidden = true), 3000); }
function status(msg, bad) { const s = $("status"); if (!msg) { s.hidden = true; return; } s.textContent = msg; s.className = "banner" + (bad ? " bad" : ""); s.hidden = false; }
function store(k, v) { try { if (v === undefined) return JSON.parse(localStorage.getItem(k) || "null"); localStorage.setItem(k, JSON.stringify(v)); } catch (_) { return null; } }

// ---------- API + outbox ----------
class HttpError extends Error { constructor(code, msg) { super(msg); this.code = code; } }
async function api(method, path, body) {
  const r = await fetch(path, { method, headers: body ? { "Content-Type": "application/json" } : {}, body: body ? JSON.stringify(body) : undefined, credentials: "same-origin" });
  if (r.status === 204) return null;
  const data = await r.json().catch(() => null);
  if (!r.ok) throw new HttpError(r.status, (data && data.error) || "Erreur " + r.status);
  return data;
}
const isNetwork = (e) => !(e instanceof HttpError) || e.code >= 500;
function outbox() { return store("muller.outbox") || []; }
function setOutbox(o) { store("muller.outbox", o); renderOutbox(); }
function enqueue(op) { setOutbox(outbox().filter((x) => x.id !== op.id).concat(op)); }
let flushing = false;
async function flush() {
  if (flushing) return; flushing = true;
  try {
    for (const op of outbox()) {
      try {
        if (op.op === "put") await api("PUT", "/api/entries/" + op.id, op.body);
        else await api("DELETE", "/api/entries/" + op.id);
        setOutbox(outbox().filter((x) => !(x.id === op.id && x.ts === op.ts)));
      } catch (e) {
        if (isNetwork(e)) { setOnline(false); return; }
        setOutbox(outbox().filter((x) => !(x.id === op.id && x.ts === op.ts)));
        status(`Un poste en attente a été refusé par le serveur : ${e.message}`, true);
      }
    }
    setOnline(true);
  } finally { flushing = false; }
}
function renderOutbox() {
  const k = outbox().length, b = $("outbox");
  b.hidden = !k; b.textContent = k ? `${k} saisie${k > 1 ? "s" : ""} en attente d'envoi — elles partiront dès le retour du réseau.` : "";
}
function setOnline(on) { state.online = on; plate(); }

// ---------- form ----------
function seg(id, key) {
  $(id).querySelectorAll("button").forEach((b) => {
    b.setAttribute("aria-pressed", b.dataset.v === state[key]);
    b.onclick = () => { state[key] = b.dataset.v; if (key === "equipe") state.equipeManual = true; if (key === "poste") { state.equipeManual = false; seg(id, key); onDate(); return; } seg(id, key); equipeHint(); };
  });
}
function equipeHint() {
  const w = isoWeek(parseDate($("f-date").value)).week, nuit = state.poste === "Nuit";
  $("equipeBox").hidden = nuit; $("nuitHint").hidden = !nuit; if (nuit) return;
  const def = teamFor(w, state.poste);
  $("equipeHint").textContent = state.equipe === def ? `S${w} (semaine ${w % 2 ? "impaire" : "paire"}) : ${def} est le régleur prévu en ${state.poste.toLowerCase()}.` : `Attention : en S${w}, le régleur prévu en ${state.poste.toLowerCase()} est ${def}.`;
}
function onDate() {
  if (!$("f-date").value) $("f-date").value = fmtDate(new Date());
  const w = isoWeek(parseDate($("f-date").value));
  $("f-week").value = "S" + w.week + " · " + w.year;
  if (state.poste === "Nuit") state.equipe = "Nuit";
  else if (!state.equipeManual || state.equipe === "Nuit") { state.equipe = teamFor(w.week, state.poste); seg("seg-equipe", "equipe"); }
  equipeHint(); plate();
}
function plate() { const w = isoWeek(new Date()); const p = $("plate"); p.textContent = "S" + w.week + " · Matin " + teamFor(w.week, "Matin") + (state.online ? "" : " · hors ligne"); p.classList.toggle("off", !state.online); }

function renderStops() {
  const box = $("stops"); box.innerHTML = "";
  state.stops.forEach((s, i) => {
    const r = document.createElement("div"); r.className = "stop";
    const sel = document.createElement("select"); sel.id = "stop-c-" + i; sel.setAttribute("aria-label", "Cause");
    CAUSES.forEach((c) => { const o = document.createElement("option"); o.textContent = c; if (c === s.cause) o.selected = true; sel.append(o); });
    sel.onchange = () => { s.cause = sel.value; };
    const m = document.createElement("input"); m.type = "number"; m.id = "stop-m-" + i; m.className = "num"; m.inputMode = "numeric"; m.min = 0; m.placeholder = "min"; m.value = s.min || ""; m.setAttribute("aria-label", "Durée en minutes"); m.oninput = () => { s.min = n(m.value); stopTotal(); };
    const x = document.createElement("button"); x.type = "button"; x.className = "iconbtn"; x.textContent = "×"; x.setAttribute("aria-label", "Retirer cet arrêt"); x.onclick = () => { state.stops.splice(i, 1); renderStops(); };
    r.append(sel, m, x); box.append(r);
  });
  stopTotal();
}
function stopTotal() { const t = state.stops.reduce((a, s) => a + n(s.min), 0); $("stopTotal").textContent = state.stops.length ? `Total arrêts : ${t} min` : "Aucun arrêt saisi pour ce poste."; }

function resetForm(keepDate) {
  state.editId = null; state.stops = []; state.equipeManual = false;
  if (!keepDate) $("f-date").value = fmtDate(new Date());
  ["f-ref", "f-ouv", "f-pcs", "f-tch", "f-com"].forEach((i) => ($(i).value = ""));
  $("f-nch").value = 0; $("formTitle").textContent = "Nouveau poste"; $("save").textContent = "Enregistrer le poste"; $("cancelEdit").hidden = true;
  onDate(); renderStops();
}
function loadEntry(e) {
  state.editId = e.id; $("f-date").value = e.date; state.poste = e.poste; state.equipe = e.equipe; state.equipeManual = true;
  $("f-ref").value = e.ref || ""; $("f-ouv").value = e.ouverture ?? ""; $("f-pcs").value = e.pieces || ""; $("f-nch").value = e.nbChangements || 0; $("f-tch").value = e.tpsChangement ?? ""; $("f-com").value = e.commentaire || "";
  state.stops = (e.arrets || []).map((s) => ({ ...s }));
  seg("seg-poste", "poste"); seg("seg-equipe", "equipe"); onDate(); renderStops();
  $("formTitle").textContent = "Modifier le poste"; $("save").textContent = "Enregistrer les modifications"; $("cancelEdit").hidden = false;
  show("saisie"); window.scrollTo(0, 0);
}

async function save() {
  const date = $("f-date").value; if (!date) { toast("Indiquez la date du poste."); return; }
  const ouv = n($("f-ouv").value); if (ouv <= 0) { toast("Indiquez les heures d'ouverture."); $("f-ouv").focus(); return; }
  const wd = parseDate(date).getDay();
  if (wd === 0) { toast("Pas de poste le dimanche. La nuit de dimanche à lundi se saisit au lundi."); return; }
  if (wd === 6 && state.poste !== "Matin") { toast("Le samedi, seul le poste du matin existe."); return; }
  const auteur = $("f-aut").value.trim(); store("muller.auteur", auteur);
  const id = state.editId || newId();
  const body = { date, poste: state.poste, equipe: state.equipe, ref: $("f-ref").value.trim(), pieces: Math.max(0, Math.round(n($("f-pcs").value))), ouverture: ouv,
    nbChangements: Math.max(0, Math.round(n($("f-nch").value))), tpsChangement: n($("f-tch").value),
    arrets: state.stops.filter((s) => n(s.min) > 0).map((s) => ({ cause: s.cause, min: n(s.min) })),
    commentaire: $("f-com").value.trim(), auteur, updatedAt: new Date().toISOString() };
  const label = state.editId ? "Poste modifié" : `Poste ${state.poste} du ${parseDate(date).toLocaleDateString("fr-FR")} enregistré`;
  const btn = $("save"); btn.disabled = true;
  try {
    await flush();
    await api("PUT", "/api/entries/" + id, body); setOnline(true); toast(label); resetForm(true);
  } catch (e) {
    if (isNetwork(e)) { enqueue({ op: "put", id, body, ts: Date.now() }); setOnline(false); toast("Gardé sur l'appareil, envoi dès le retour du réseau"); resetForm(true); }
    else toast(e.message);
  } finally { btn.disabled = false; loadRefs(); if (!$("view-semaine").hidden) loadWeek(); }
}

// ---------- week view ----------
function show(v) {
  const s = v === "saisie";
  $("view-saisie").hidden = !s; $("view-semaine").hidden = s;
  $("tab-saisie").setAttribute("aria-selected", s); $("tab-semaine").setAttribute("aria-selected", !s);
  if (!s) { renderWeek(); loadWeek(); }
}
function weekKey() { return `muller.week.${state.viewWeek.year}.${state.viewWeek.week}`; }
async function loadWeek() {
  const { year, week } = state.viewWeek, key = weekKey();
  try { const es = await api("GET", `/api/entries?year=${year}&week=${week}`); if (key !== weekKey()) return; state.week = es; store(key, es); setOnline(true); }
  catch (e) { if (isNetwork(e)) setOnline(false); else status(e.message, true); }
  renderWeek();
}
// Entries of the viewed week, with outbox changes applied on top.
function weekEntries() {
  const { year, week } = state.viewWeek; const byId = new Map(state.week.map((e) => [e.id, e]));
  for (const op of outbox()) {
    if (op.op === "del") { byId.delete(op.id); continue; }
    const w = isoWeek(parseDate(op.body.date));
    if (w.year === year && w.week === week) byId.set(op.id, { ...op.body, id: op.id, week: w.week, year: w.year, pending: true });
    else byId.delete(op.id);
  }
  return [...byId.values()];
}
function el(tag, cls, text) { const x = document.createElement(tag); if (cls) x.className = cls; if (text != null) x.textContent = text; return x; }
function renderWeek() {
  const { year, week } = state.viewWeek;
  $("wLabel").innerHTML = ""; $("wLabel").append("S" + week, el("span", "", `${year} · matin ${teamFor(week, "Matin")} · AM ${teamFor(week, "Après-midi")}`));
  $("export").href = `/api/export?year=${year}&week=${week}`;
  const es = weekEntries(), sum = (k) => es.reduce((a, e) => a + n(e[k]), 0);
  const stopsMin = es.reduce((a, e) => a + (e.arrets || []).reduce((b, s) => b + n(s.min), 0), 0);
  const tiles = [["Postes saisis", es.length], ["Heures d'ouverture", fmtH(sum("ouverture")) + " h"], ["Pièces produites", sum("pieces").toLocaleString("fr-FR")], ["Changements de série", sum("nbChangements") + " · " + sum("tpsChangement") + " min"], ["Arrêts déclarés", stopsMin + " min"]];
  $("tiles").replaceChildren(...tiles.map(([l, v]) => { const t = el("div", "tile"); t.append(el("small", "", l), el("b", "", v)); return t; }));

  const by = {}; es.forEach((e) => (e.arrets || []).forEach((s) => (by[s.cause] = (by[s.cause] || 0) + n(s.min))));
  if (sum("tpsChangement") > 0) by["Changements de série"] = (by["Changements de série"] || 0) + sum("tpsChangement");
  const rows = Object.entries(by).sort((a, b) => b[1] - a[1]), max = rows[0] ? rows[0][1] : 1, par = $("pareto");
  par.replaceChildren();
  if (!rows.length) par.append(el("p", "empty", "Aucun arrêt déclaré cette semaine."));
  rows.forEach(([c, m]) => { const d = el("div", "bar"), t = el("div", "track"), f = el("div", "fill"); f.style.width = Math.max(3, (m / max) * 100) + "%"; t.append(f); d.append(el("span", "", c), el("span", "v", m + " min"), t); par.append(d); });

  const list = $("list"); list.replaceChildren();
  if (!es.length) { const em = el("div", "empty"); em.append(el("strong", "", `Aucun poste en S${week}`), "Les postes enregistrés dans l'onglet Saisie apparaissent ici, jour par jour."); list.append(em); return; }
  const ord = { "Matin": 0, "Après-midi": 1, "Nuit": 2 }, days = {};
  es.forEach((e) => (days[e.date] = days[e.date] || []).push(e));
  Object.keys(days).sort().forEach((d) => {
    const sec = el("div", "day"); sec.append(el("h3", "", parseDate(d).toLocaleDateString("fr-FR", { weekday: "long", day: "numeric", month: "long" })));
    days[d].sort((a, b) => ord[a.poste] - ord[b.poste]).forEach((e) => {
      const it = el("div", "entry"), st = (e.arrets || []).reduce((b, s) => b + n(s.min), 0);
      const top = el("div", "top"), l = el("div");
      l.append(el("span", "chip", e.poste)); if (e.equipe !== "Nuit") l.append(el("span", "chip" + (e.equipe === "MR" ? " mr" : ""), e.equipe));
      if (e.pending) l.append(el("span", "chip wait", "en attente"));
      l.append(el("strong", "", e.ref || "Réf. non précisée"));
      top.append(l, el("div", "meta num", `${fmtH(e.ouverture)} h ouv.${e.pieces ? " · " + e.pieces.toLocaleString("fr-FR") + " pcs" : ""}`));
      it.append(top, el("div", "meta", `${e.nbChangements || 0} chgt (${e.tpsChangement || 0} min) · arrêts ${st} min${e.auteur ? " · saisi par " + e.auteur : ""}`));
      if (e.commentaire) it.append(el("div", "meta", "« " + e.commentaire + " »"));
      const a = el("div", "actions");
      if (state.confirmDel === e.id) {
        const y = el("button", "danger", "Supprimer"), no = el("button", "secondary", "Garder");
        y.onclick = () => del(e.id); no.onclick = () => { state.confirmDel = null; renderWeek(); };
        a.append(el("span", "meta", "Supprimer ce poste ?"), y, no);
      } else {
        const ed = el("button", "secondary", "Modifier"), de = el("button", "secondary", "Supprimer");
        ed.onclick = () => loadEntry(e); de.onclick = () => { state.confirmDel = e.id; renderWeek(); };
        a.append(ed, de);
      }
      it.append(a); sec.append(it);
    });
    list.append(sec);
  });
}
async function del(id) {
  state.confirmDel = null;
  try { await api("DELETE", "/api/entries/" + id); setOutbox(outbox().filter((x) => x.id !== id)); toast("Poste supprimé"); }
  catch (e) { if (isNetwork(e)) { enqueue({ op: "del", id, ts: Date.now() }); setOnline(false); toast("Suppression gardée, envoi dès le retour du réseau"); } else toast(e.message); }
  state.week = state.week.filter((x) => x.id !== id); renderWeek(); loadWeek();
}
function shiftWeek(dx) {
  const { year, week } = state.viewWeek, jan4 = new Date(year, 0, 4), mon = new Date(jan4);
  mon.setDate(jan4.getDate() - ((jan4.getDay() || 7) - 1) + (week - 1) * 7 + dx * 7);
  state.viewWeek = isoWeek(mon); state.week = store(weekKey()) || []; state.confirmDel = null; renderWeek(); loadWeek();
}
async function loadRefs() {
  let refs = store("muller.refs") || [];
  try { refs = await api("GET", "/api/refs"); store("muller.refs", refs); } catch (_) {}
  $("refs").replaceChildren(...refs.map((r) => { const o = document.createElement("option"); o.value = r; return o; }));
}

// ---------- wiring ----------
$("f-date").addEventListener("change", onDate);
$("nch-minus").onclick = () => { $("f-nch").value = Math.max(0, n($("f-nch").value) - 1); };
$("nch-plus").onclick = () => { $("f-nch").value = n($("f-nch").value) + 1; };
$("addStop").onclick = () => { state.stops.push({ cause: CAUSES[0], min: 0 }); renderStops(); const m = $("stop-m-" + (state.stops.length - 1)); m && m.focus(); };
$("save").onclick = save; $("cancelEdit").onclick = () => resetForm();
$("tab-saisie").onclick = () => show("saisie"); $("tab-semaine").onclick = () => show("semaine");
$("wPrev").onclick = () => shiftWeek(-1); $("wNext").onclick = () => shiftWeek(1);
$("export").addEventListener("click", (ev) => { if (!state.online) { ev.preventDefault(); toast("Export disponible uniquement en ligne."); } });
window.addEventListener("online", flush);
setInterval(() => { if (document.hidden) return; flush(); if (!$("view-semaine").hidden) loadWeek(); }, 20000);
document.addEventListener("visibilitychange", () => { if (!document.hidden) { flush(); if (!$("view-semaine").hidden) loadWeek(); } });

state.viewWeek = isoWeek(new Date()); state.week = store(weekKey()) || [];
$("f-aut").value = store("muller.auteur") || "";
seg("seg-poste", "poste"); resetForm(); renderOutbox(); loadRefs(); flush();
if ("serviceWorker" in navigator) navigator.serviceWorker.register("sw.js").catch(() => {});
