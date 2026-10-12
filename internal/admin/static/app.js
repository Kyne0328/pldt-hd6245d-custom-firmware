"use strict";

// HGFW Stage 2 console: observation only. Never interpolate API values into HTML.
const $ = (id) => document.getElementById(id);
const navNames = {overview: "Overview", network: "Network", storage: "USB storage", services: "Services"};
let latest = null;
let networkFilter = "all";
let inFlight = false;
let lastUpdate = null;
const hasArray = (value) => Array.isArray(value) ? value : [];
const valueText = (id, value) => { const el = $(id); if (el) el.textContent = String(value); };
const formatBytes = (bytes) => {
  const n = Number(bytes);
  if (!Number.isFinite(n) || n < 0) return "Unknown size";
  if (n === 0) return "0 B";
  const u = ["B","KiB","MiB","GiB","TiB"];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), u.length - 1);
  return (i === 0 ? String(n) : (n / Math.pow(1024,i)).toFixed(1)) + " " + u[i];
};
const readableUptime = (seconds) => {
  if (!Number.isFinite(Number(seconds)) || Number(seconds) < 0) return "—";
  const v = Math.floor(Number(seconds));
  const days = Math.floor(v / 86400), hours = Math.floor(v % 86400 / 3600), mins = Math.floor(v % 3600 / 60);
  return days ? days + "d " + hours + "h" : hours ? hours + "h " + mins + "m" : mins + "m";
};
const isUp = (iface) => String(iface.flags || "").split("|").includes("up");
const isRunning = (iface) => String(iface.flags || "").split("|").includes("running");
const ifaceType = (name) => {
  if (/^wl[0-9]/.test(name)) return "wireless";
  if (/^(eth|wan|bcmsw)/.test(name)) return "ethernet";
  return "system";
};
const radioDevice = (iface) => /^wl[0-9]+$/.test(String(iface.name));
const ip4 = (iface) => hasArray(iface.addresses).find(a => /^\d{1,3}(\.\d{1,3}){3}\//.test(String(a))) || "";
function el(tag, cls, text) {
  const node = document.createElement(tag);
  if (cls) node.className = cls;
  if (text !== undefined && text !== null) node.textContent = String(text);
  return node;
}
function setPage() {
  const name = location.hash.slice(1);
  const chosen = Object.prototype.hasOwnProperty.call(navNames, name) ? name : "overview";
  document.querySelectorAll(".view").forEach(view => { view.hidden = view.dataset.view !== chosen; });
  document.querySelectorAll("[data-nav]").forEach(link => {
    const selected = link.dataset.nav === chosen;
    link.classList.toggle("active", selected);
    if (selected) link.setAttribute("aria-current","page");
    else link.removeAttribute("aria-current");
  });
  valueText("crumb-current", navNames[chosen]);
  document.title = "HGFW Console · " + navNames[chosen];
  if (latest && chosen === "network") renderInterfaceTable(latest.device.interfaces);
  window.scrollTo({top:0,behavior:"instant"});
}
function statusLine(text, kind) {
  const state = $("sync-state");
  state.querySelector(".status-beacon").className = "status-beacon" + (kind === "error" ? " offline" : kind === "loading" ? " loading" : "");
  valueText("sync-label", text);
  state.title = kind === "error" ? "Not receiving fresh API data" : text;
}
function notice(message, variant) {
  $("alert").className = "notice" + (variant ? " " + variant : "");
  valueText("alert", message);
}
function badge(text, style) { return el("span","state-pill" + (style ? " " + style : ""),text); }
function compactRow(name, subtitle, state, kind) {
  const row = el("div","compact-item");
  const main = el("div","compact-main");
  const icon = el("span","compact-icon", kind === "wireless" ? "◉" : kind === "storage" ? "▣" : "⇄");
  icon.setAttribute("aria-hidden","true");
  const info = el("div","");
  info.append(el("strong","",name),el("small","",subtitle));
  main.append(icon,info);
  row.append(main,badge(state,state.toLowerCase()));
  return row;
}
function emptyState(message, detail, glyph) {
  const box = el("div","empty-state");
  const icon = el("span","empty-symbol",glyph || "◌");
  icon.setAttribute("aria-hidden","true");
  box.append(icon,el("strong","",message),el("p","",detail));
  return box;
}
function renderOverview(d, services) {
  const interfaces = hasArray(d.interfaces);
  const disks = hasArray(d.disks);
  const radios = interfaces.filter(radioDevice);
  const drives = disks.filter(d => !d.partition);
  const bridge = interfaces.find(i=>i.name==="br0") || interfaces.find(i=>ip4(i) && /^br/.test(i.name)) || interfaces.find(i=>ip4(i) && i.name!=="lo");
  const percent = d.memTotalKB > 0 ? Math.max(0,Math.min(100,(1 - d.memAvailableKB / d.memTotalKB)*100)) : null;
  valueText("hero-bridge-name",bridge ? bridge.name : "No bridge");
  valueText("hero-bridge-state",bridge ? (isUp(bridge) ? "Kernel interface up" : "Interface not up") : "Not detected");
  valueText("hero-address",bridge && ip4(bridge) ? ip4(bridge) : "No IPv4 detected");
  valueText("hero-radio-number",radios.length + (radios.length === 1 ? " radio" : " radios"));
  valueText("metric-interfaces",interfaces.length);
  valueText("metric-radios",radios.length);
  valueText("metric-disks",drives.length);
  valueText("metric-services",services.length);
  valueText("nav-network-count",interfaces.length);
  valueText("memory-percent",percent === null ? "—" : Math.round(percent) + "%");
  valueText("mem-used", d.memTotalKB > 0 ? formatBytes((d.memTotalKB-d.memAvailableKB)*1024) : "—");
  valueText("mem-free",d.memTotalKB > 0 ? formatBytes(d.memAvailableKB*1024) : "—");
  $("memory-ring").style.background = percent === null ? "#edf1f5" : "conic-gradient(#21aa92 0deg "+(percent*3.6).toFixed(1)+"deg,#edf1f5 "+(percent*3.6).toFixed(1)+"deg 360deg)";
  $("memory-ring").setAttribute("aria-label",percent === null ? "Memory use unknown" : Math.round(percent) + " percent memory in use");
  valueText("uptime",readableUptime(d.uptimeSeconds));
  valueText("load",Number.isFinite(Number(d.loadOne)) ? Number(d.loadOne).toFixed(2) : "—");
  const quick = $("quick-interfaces");
  const candidates = ["br0","wl0","wl1","eth0.0","eth3.0","eth0","eth1"];
  const chosen = candidates.map(name=>interfaces.find(i=>i.name===name)).filter(Boolean).slice(0,4);
  if (!chosen.length) chosen.push(...interfaces.filter(i=>i.name!=="lo").slice(0,4));
  const rows = chosen.map(i=>compactRow(i.name,ip4(i) || (ifaceType(i.name)==="wireless" ? "Wireless interface" : "No IPv4 address"),isUp(i) ? "Up" : "Down",ifaceType(i.name)));
  if (!rows.length) rows.push(emptyState("No network interfaces","The Linux kernel hasn't reported any interfaces.","⇄"));
  quick.replaceChildren(...rows);
  const qstorage = $("quick-storage");
  const diskRows = drives.map(d=>compactRow(d.device || d.name,formatBytes(d.bytes)+" · USB mass storage", "Detected", "storage"));
  if (!diskRows.length) diskRows.push(emptyState("No USB disks detected","Connect a compatible USB drive to show it here.","▣"));
  qstorage.replaceChildren(...diskRows);
}
function renderNetwork(d) {
  const interfaces = hasArray(d.interfaces);
  const inspection = d.network || {};
  const routes = hasArray(inspection.ipv4Routes);
  const evidence = hasArray(inspection.interfaces);
  const bridge = interfaces.find(i=>i.name==="br0") || interfaces.find(i=>/^br/.test(i.name));
  valueText("net-bridge",bridge ? bridge.name : "—");
  valueText("net-bridge-ip",bridge ? (ip4(bridge)||"No IPv4 address") : "No bridge detected");
  valueText("net-radios",interfaces.filter(radioDevice).length);
  valueText("net-default-route",inspection.defaultInterface || "No default");
  valueText("net-default-gateway",inspection.defaultGateway ? "Gateway: "+inspection.defaultGateway : "No IPv4 gateway reported");
  valueText("route-count",routes.length + " routes");
  const routeNodes = routes.map(r=>compactRow(r.default?"Default IPv4 route":r.destination+"/"+r.mask,
    (r.interface||"unknown")+" · "+(r.gateway?"via "+r.gateway:"on-link") + " · metric "+r.metric,
    r.default?"Default":"Kernel","ethernet"));
  $("route-list").replaceChildren(...(routeNodes.length?routeNodes:[emptyState("No IPv4 routes detected","The route file may be absent or empty.","⇄")]));
  const notes=hasArray(inspection.notes).slice(0,4).map(t=>el("p","evidence-note",t));
  $("role-notes").replaceChildren(...notes);
  renderInterfaceTable(interfaces);
}
function renderInterfaceTable(interfaces) {
  const list = hasArray(interfaces);
  const query = $("interface-search").value.trim().toLowerCase();
  const evidence = new Map(hasArray(latest?.device?.network?.interfaces).map(e=>[e.name,e]));
  const visible = list.filter(i => {
    if (networkFilter !== "all" && (evidence.get(i.name)?.kind || ifaceType(i.name))!==networkFilter) return false;
    return !query || [i.name,i.mac,evidence.get(i.name)?.role,evidence.get(i.name)?.operState,...hasArray(i.addresses)].join(" ").toLowerCase().includes(query);
  });
  valueText("interface-visible-count",visible.length + " shown");
  valueText("network-description",list.length + " interfaces reported by Linux.");
  const rows = visible.map(i=>{
    const row = el("tr");
    const nameCell = el("td");
    const e=evidence.get(i.name)||{};
    nameCell.append(el("span","if-name",i.name),el("span","if-type",e.role || ifaceType(i.name).toUpperCase()));
    if(e.roleNote)nameCell.title=e.roleNote + " · confidence: " + (e.confidence || "unknown");
    const stateCell = el("td");stateCell.append(badge(isUp(i) ? "Up" : "Down",isUp(i)?"up":"down"));
    const linkLabel=e.carrier===true?"Carrier detected":e.carrier===false?"No carrier":"Carrier unknown";
    stateCell.append(el("span","if-type",linkLabel+(e.operState?" · "+e.operState:"")+(e.master?" · master "+e.master:"")));
    const ipCell = el("td","code-text",hasArray(i.addresses).length ? i.addresses.join(" · ") : "—");
    const macCell = el("td","code-text",i.mac || "—");
    const trafficCell = el("td","code-text",formatBytes(e.rxBytes||0)+" / "+formatBytes(e.txBytes||0));
    const mtuCell = el("td","code-text",i.mtu || "—");
    row.append(nameCell,stateCell,ipCell,macCell,trafficCell,mtuCell);
    return row;
  });
  if (!rows.length) {const row=el("tr");const cell=el("td","table-empty",query || networkFilter!=="all"?"No matching interfaces. Try another search or filter.":"No interfaces reported.");cell.colSpan=6;row.append(cell);rows.push(row);}
  $("interface-rows").replaceChildren(...rows);
}
function renderStorage(disks) {
  const list = hasArray(disks), drives = list.filter(d=>!d.partition), parts=list.filter(d=>d.partition);
  valueText("store-disks",drives.length);
  valueText("store-partitions",parts.length);
  valueText("store-mounted",parts.filter(d=>d.mountpoint).length);
  valueText("disk-total-count",list.length + " entries");
  const rows = list.map(d=>{
    const row=el("div","storage-entry");
    const left=el("div","storage-left");
    const icon=el("span","drive-icon",d.partition?"◧":"▣");icon.setAttribute("aria-hidden","true");
    const info=el("div","");
    info.append(el("strong","",d.device || d.name),el("small","",d.partition?"USB partition": "USB disk"));
    if(d.mountpoint)info.append(el("small","", "Mounted at "+d.mountpoint));
    left.append(icon,info);
    const side=el("div","storage-meta");
    side.append(el("strong","",formatBytes(d.bytes)),badge(d.mountpoint?"Mounted":"Detected",d.mountpoint?"up":""));
    row.append(left,side);return row;
  });
  if(!rows.length)rows.push(emptyState("Nothing plugged in yet","Connect a USB storage device to display disks and partitions.","▣"));
  $("storage-devices").replaceChildren(...rows);
}
function renderServices(services) {
  const list=hasArray(services);
  valueText("svc-total",list.length);
  valueText("svc-running",list.filter(s=>s.state==="running").length);
  valueText("svc-restarts",list.reduce((acc,s)=>acc+(Number(s.restarts)||0),0));
  valueText("service-total-count",list.length+" services");
  const rows=list.map(s=>{
    const row=el("div","service-entry");const left=el("div","service-left");
    const icon=el("span","service-icon","≡");icon.setAttribute("aria-hidden","true");
    const info=el("div","");
    info.append(el("strong","",s.name),el("small","","PID "+(s.pid||"—")+" · "+(s.restarts||0)+" restart attempts"));
    if(s.lastError)info.append(el("small","",s.lastError));
    left.append(icon,info);
    const side=el("div","service-meta");side.append(badge(s.state||"unknown",s.state));
    row.append(left,side);return row;
  });
  if(!rows.length) rows.push(emptyState("No services configured","The service supervisor is idle. No extra processes are started in the default development build.","≡"));
  $("service-devices").replaceChildren(...rows);
}
async function refresh() {
  if(inFlight)return;
  inFlight=true;
  $("refresh").disabled=true;
  if(!latest)statusLine("Connecting","loading");
  try{
    const response=await fetch("/api/status",{credentials:"same-origin",cache:"no-store"});
    if(response.status===401){location.assign("/login");return;}
    if(!response.ok)throw Error("HTTP "+response.status);
    const data=await response.json();
    if(!data || !data.device || !Array.isArray(data.device.interfaces) || !Array.isArray(data.device.disks))throw Error("Unexpected device response");
    latest=data;
    const services=hasArray(data.services);
    renderOverview(data.device,services);
    renderNetwork(data.device);
    renderStorage(data.device.disks);
    renderServices(services);
    lastUpdate=new Date();
    statusLine("Live","ok");
    valueText("updated","Updated "+lastUpdate.toLocaleTimeString());
    notice("","ok");
  }catch(error){
    statusLine("Offline","error");
    notice("Couldn't refresh hardware data. "+error.message+". Previously displayed readings may be stale.","error");
  }finally{inFlight=false;$("refresh").disabled=false;}
}
async function signOut(){
  const button=$("logout");
  button.disabled=true;
  try{
    const req=await fetch("/api/csrf",{credentials:"same-origin",cache:"no-store"});
    if(!req.ok)throw Error("Cannot get sign-out token");
    const data=await req.json();
    const response=await fetch("/logout",{method:"POST",credentials:"same-origin",headers:{"X-CSRF-Token":data.csrf}});
    if(!response.ok)throw Error("Sign-out failed");
    location.assign("/login");
  }catch(error){button.disabled=false;notice(error.message,"error");}
}
window.addEventListener("hashchange",setPage);
$("refresh").addEventListener("click",refresh);
$("logout").addEventListener("click",signOut);
$("interface-search").addEventListener("input",()=>{if(latest)renderInterfaceTable(latest.device.interfaces);});
document.querySelectorAll("[data-filter]").forEach(button=>button.addEventListener("click",()=>{
  networkFilter=button.dataset.filter;
  document.querySelectorAll("[data-filter]").forEach(b=>{const current=b===button;b.classList.toggle("active",current);b.setAttribute("aria-pressed",String(current));});
  if(latest)renderInterfaceTable(latest.device.interfaces);
}));
document.addEventListener("visibilitychange",()=>{if(!document.hidden && lastUpdate && Date.now()-lastUpdate.getTime()>15000)refresh();});
setPage();
refresh();
setInterval(()=>{if(!document.hidden)refresh();},15000);
