"use strict";

// HGFW Stage 2 console: observation only. Never interpolate API values into HTML.
const $ = (id) => document.getElementById(id);
const navNames = {overview: "Overview", status: "Status", network: "Network", security: "Security", applications: "Applications", management: "Management", storage: "USB storage", services: "Services"};
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
  const name = location.hash.slice(1).split("/")[0];
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
  routeCatalog();
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
// Original PLDT variant menu used only as an offline reference. This is
// independent HGFW UI code; no vendor ASP/CGI endpoints are invoked.
const featureCatalog = {
 status:[
  ["device","Device information","Live observation","Kernel, model, uptime and memory",d=>[["Model",d.model],["Kernel",d.kernel],["Uptime",readableUptime(d.uptimeSeconds)],["Available memory",formatBytes((d.memAvailableKB||0)*1024)]]],
  ["wifi24","2.4 GHz wireless status","Read-only driver probe","Radio link and association data from the driver only if a supported read-only helper is available",d=>wirelessEvidence(d,"wl0")],
  ["wifi5","5 GHz wireless status","Read-only driver probe","Radio link and association data from the driver only if a supported read-only helper is available",d=>wirelessEvidence(d,"wl1")],
  ["wifi-clients","Wi-Fi clients list","Read-only driver probe","Connected clients are listed ONLY when a supported native Broadcom wl or nl80211 iw association query succeeds. ARP neighbors are not Wi-Fi clients.",d=>wirelessClientFacts(d)],
  ["lan-ports","LAN port status","Live observation","LAN 1–4 carrier and bridge observations with operator-provided mapping",d=>[["Port map",hasArray(d.lanPorts).length+" currently matched"],["Bridge",bridgeText(d)]]],
  ["dhcp-clients","DHCP client list","Backend pending","Lease records are not independently verified. Wi-Fi MACs may be correlated to kernel ARP observations, but these are NOT DHCP leases",d=>[["Standard dnsmasq lease file",d.dhcpLeases?.available?"Present (only validated entries considered)":"Unavailable; stock DHCP source unverified"],["Wireless lease host labels",d.dhcpLeases?.matchedWirelessNames||0],["Observed ARP entries",hasArray(d.arpNeighbors).length+" kernel neighbors"],["Wi-Fi/ARP links",hasArray(d.wireless?.radios).reduce((n,r)=>n+hasArray(r.stations).filter(s=>Boolean(s.neighborIPv4)).length,0)+" matched by MAC; not leases"]]],
  ["neighbors","LAN neighbor table","Live observation","Completed IPv4 ARP entries; not a DHCP lease or Wi-Fi association list",d=>[["IPv4 ARP entries",hasArray(d.arpNeighbors).length],...hasArray(d.arpNeighbors).slice(0,16).map(n=>[n.address,n.mac+" · "+n.interface])]],
  ["wan-status","WAN status","Kernel routes only","Default route observation is not proof of an upstream connection",d=>[["Default route",d.network?.defaultInterface||"None reported"],["Gateway",d.network?.defaultGateway||"None reported"]]],
  ["optical","Optical status","Interface detected only","GPON/OMCI interface metadata; optical RX/TX power not exposed by this backend",d=>[["Optical interface",hasArray(d.interfaces).filter(i=>/^(sfu|gpon|veip)/.test(i.name)).map(i=>i.name).join(", ")||"Not found"],["Optical power","Not measured"]]],
  ["voip","VoIP status","Interface detected only","VoIP interface presence is not voice registration or call capability",d=>[["VoIP-related interfaces",hasArray(d.interfaces).filter(i=>/^(voip|tel[0-9])/.test(i.name)).map(i=>i.name).join(", ")||"Not found"],["SIP registration","Not inspected"]]],
 ],
 network:[
  ["wifi-config-24","2.4 GHz wireless settings","Read-only driver probe","Channel and SSID details are observed when available; AP/STA configuration writes remain disabled",d=>wirelessEvidence(d,"wl0")],
  ["wifi-config-5","5 GHz wireless settings","Read-only driver probe","5 GHz SSID/channel observations are read-only; repeater/client configuration remains disabled",d=>wirelessEvidence(d,"wl1")],
  ["wifi-security","Wi-Fi security","Not enabled","WPA2/WPA3 credential handling, guest SSIDs and authentication changes are not available",()=>[]],
  ["wifi-power-24","2.4 GHz transmit power","Not enabled","Radio transmit power controls need verified Broadcom driver support",()=>[]],
  ["wifi-power-5","5 GHz transmit power","Not enabled","5 GHz radio power control remains unsupported by HGFW",()=>[]],
  ["wps","Wi-Fi Protected Setup","Not enabled","WPS status and configuration cannot be verified and are not exposed for activation",()=>[]],
  ["lan-config","LAN & DHCP","Not enabled","The independent network-plan validator works offline; DHCP configuration is not applied",d=>[["Bridge",bridgeText(d)],["IPv4 forwarding",triState(d.platform?.ipv4Forwarding)]]],
  ["wan-config","Broadband / WAN","Not enabled","WAN interface binding and connectivity must be tested with verified recovery",d=>[["Kernel default route",d.network?.defaultInterface||"None reported"]]],
  ["dns-resolvers","Linux DNS resolvers","Read-only observation","Observed Linux resolver file, not a validated upstream, DHCP DNS option or an active HGFW DNS daemon",d=>[["Resolver file",d.resolvers?.available?"Readable":"Unavailable"],["Name servers",hasArray(d.resolvers?.nameservers).join(", ")||"None exposed"],["Independent DNS engine","Static loopback responder self-tested; LAN DNS not enabled"]]],
  ["iptv","IPTV & VLANs","Not enabled","Physical switch VLAN assignment is unverified on this hardware variant",()=>[]],
  ["ipv6","IPv6","Kernel flag only","IPv6 routing and firewall configuration is not enabled",d=>[["IPv6 forwarding",triState(d.platform?.ipv6Forwarding)]]],
  ["remote-isp","Remote management (TR-069)","Not enabled","Vendor provisioning and ISP access controls are intentionally excluded",()=>[]],
  ["ont-auth","Optical / OLT authentication","Not enabled","GPON, PON identity and OMCI settings are outside this independently validated configuration scope",()=>[]],
 ],
 security:[
  ["firewall","Firewall status","Kernel metadata only","Read kernel table names and forwarding flags, not full effective firewall rules",d=>[["IPv4 forwarding",triState(d.platform?.ipv4Forwarding)],["IPv4 tables",hasArray(d.platform?.ipv4FirewallTables).join(", ")||"None exposed"],["IPv6 tables",hasArray(d.platform?.ipv6FirewallTables).join(", ")||"None exposed"]]],
  ["ip-filter","IP address filtering","Not enabled","Rule inspection/editing is not connected to iptables or the vendor firewall",()=>[]],
  ["url-filter","URL filtering","Not enabled","DNS- or HTTP-layer filtering services have not been deployed",()=>[]],
  ["mac-filter","MAC filtering","Not enabled","Bridge/switch access-control rules must not be changed without rollback",()=>[]],
  ["dhcp-filter","DHCP filtering","Not enabled","DHCP snooping and filtering require verified switch support",()=>[]],
  ["ipv6-mac-filter","IPv6 MAC filtering","Not enabled","No independently implemented IPv6 bridge filtering is available",()=>[]],
  ["acl","Access control lists","Not enabled","Access-control rule editing remains blocked without verified rollback",()=>[]],
  ["port-scan","Port scan protection","Not enabled","The stock intrusion-detection switches are not connected to HGFW",()=>[]],
  ["parental","Parental controls","Not enabled","Client policies and enforcement services are not implemented",()=>[]],
  ["wan-acl","WAN access control","Not enabled","Remote-management exposure remains blocked in HGFW preview",()=>[]],
  ["ddos","DoS protection","Not enabled","Stock DoS settings have not been reconstructed into independent rules",()=>[]],
  ["qos","Quality of service","Not enabled","Traffic queues and rate limits cannot be applied safely yet",()=>[]],
  ["remote","Remote access","Read-only preview","HGFW currently serves a temporary authenticated HTTPS preview with a strict timeout",()=>[["Network configuration","Not modified"],["Remote WAN management","Not enabled"]]],
 ],
 applications:[
  ["vpn","VPN overview","Prerequisites observed","Separate VPN server/client support from stock GRE/IPsec pass-through",d=>vpnFacts(d)],
  ["vpn-gre","VPN pass-through (GRE)","Stock feature only","Original PLDT uses a /goform/setVPNEnable pass-through setting. HGFW does not read or toggle its current value",()=>[["GRE pass-through","Current stock setting not inspected"],["VPN tunnel server","Not implied"]]],
  ["vpn-ipsec","IPsec pass-through","Stock feature only","Original stock page uses /goform/setIpsecEnable, not an independent IPsec VPN server",()=>[["IPsec pass-through","Current stock setting not inspected"],["IPsec server","Not implemented"]]],
  ["wireguard","WireGuard VPN","Prerequisites observed","Kernel module or userspace implementation would need validated TUN support and secure key storage",d=>vpnFacts(d)],
  ["openvpn","OpenVPN","Prerequisites observed","An independent OpenVPN client/server needs TUN, TLS, isolation, routing and credentials",d=>vpnFacts(d)],
  ["ddns","Dynamic DNS","Not enabled","Provider authentication, updater daemon and configuration management are not deployed",()=>[]],
  ["port-forward","Port forwarding","Not enabled","No NAT or firewall changes are permitted from the preview",()=>[]],
  ["port-trigger","Port triggering","Not enabled","Dynamic NAT rule management is not implemented",()=>[]],
  ["nat","NAT","Kernel metadata only","Forwarding flags are observable, but active NAT rule correctness is not established",d=>[["IPv4 forwarding",triState(d.platform?.ipv4Forwarding)],["NAT table",hasArray(d.platform?.ipv4FirewallTables).includes("nat")?"Table name exposed":"Not exposed"]]],
  ["upnp","UPnP","Not enabled","No UPnP Internet Gateway Device service is running under HGFW",()=>[]],
  ["dmz","DMZ","Not enabled","No exposed-host forwarding controls are available",()=>[]],
  ["samba","Samba file sharing","Storage observation only","USB disks can be enumerated; HGFW has not deployed an SMB server",d=>[["USB disks",hasArray(d.disks).filter(d=>!d.partition).length],["SMB server","Not deployed"]]],
  ["usb-printer","USB print server","Not enabled","USB printer identification and a print spooler are not implemented",()=>[]],
  ["diagnostics","Network diagnostics","Read-only observation","Interface counters, routes and the supervised loopback health test are available; no active WAN probing is performed",d=>[["Kernel routes",hasArray(d.network?.ipv4Routes).length],["Loopback service test","hgfw network-selftest (CLI)"]]],
 ],
 management:[
  ["account","Administrator session","Active HGFW authentication","Password-hashed login, secure cookies, CSRF checks and explicit logout already work",()=>[["Current session","Authenticated"],["Device configuration writes","Disabled"]]],
  ["password","Password management","Not enabled","The temporary preview uses a one-time in-RAM password; authenticated sessions can be signed out, but credential editing is not enabled",()=>[]],
  ["logs","System logs","Not enabled","The development console does not scrape potentially sensitive stock logs",()=>[]],
  ["clock","Device clock / NTP","Not enabled","Time synchronization changes would affect TLS, logging and running processes",()=>[]],
  ["backup","Configuration backup","Not enabled","Stock configuration can contain credentials. No download or restore is exposed",()=>[]],
  ["restore","Factory restore","Blocked for safety","Factory reset would erase settings and may eliminate access to the device",()=>[]],
  ["firmware","Firmware update","Blocked for safety","No flash writes before independently verified software-only recovery",()=>[]],
  ["reboot","Reboot","Blocked for safety","Reboot operations are intentionally unavailable in this development build",()=>[]],
  ["ftp","FTP management","Not enabled","Plaintext FTP management is not offered; future secure file services require isolation",()=>[]],
  ["services","HGFW service supervisor","Read-only observation","Services explicitly launched by HGFW are shown in the Services page",()=>[["Supervisor","Running independently in HGFW process"],["Go to","Services navigation page"]]],
 ],
};
function triState(value) {return value===true?"Enabled (kernel flag)":value===false?"Disabled (kernel flag)":"Not readable";}
function bridgeText(d) {const br=hasArray(d.interfaces).find(i=>i.name==="br0");return br? br.name+" · "+(ip4(br)||"No IPv4"):"Not detected";}
function wirelessEvidence(d,name) {
 const iface=hasArray(d.interfaces).find(i=>i.name===name);
 const evidence=hasArray(d.network?.interfaces).find(i=>i.name===name);
 const radio=hasArray(d.wireless?.radios).find(r=>r.interface===name);
 const availability=radio?.associationStatus==="observed"?"Verified driver query": "Unavailable — no supported driver association query";
 return [["Kernel interface",iface?"Detected":"Not detected"],["Driver operstate",evidence?.operState||"Unknown"],["Carrier",evidence?.carrier===true?"Reported":evidence?.carrier===false?"Not reported":"Unknown"],["Bridge master",evidence?.master||"Not observed"],
 ["SSID",radio?.observedSSID||"Not available (no driver response)"],["Channel",radio?.channel||"Not available"],["Operating mode",radio?.mode||"Not verified"],["Association inventory",availability],
 ["Associated clients",radio?.associationStatus==="observed"?String(hasArray(radio.stations).length):"Unknown (not zero)"],["Data source",radio?.source||"Kernel interface only"]];
}
function wirelessClientFacts(d){
 const radios=hasArray(d.wireless?.radios);
 const result=[];
 if(!radios.length)return [["Wireless drivers","No supported radio interfaces found"],["Associated clients","Unknown"]]; 
 for(const r of radios){
  const observed=r.associationStatus==="observed";
  const clients=hasArray(r.stations);
  result.push([r.label+" / "+r.interface,observed?clients.length+" associated station(s)":"Driver query unavailable"]);
  if(observed){
   for(const st of clients.slice(0,32)){
    const signal=st.signalDBm==null?"signal not reported":st.signalDBm+" dBm";
    const rates=[st.rxBitrate&&"RX "+st.rxBitrate,st.txBitrate&&"TX "+st.txBitrate].filter(Boolean).join(" · ");
    const neighbor=st.neighborIPv4?" · ARP IPv4 "+st.neighborIPv4+" (kernel neighbor, NOT DHCP)":" · IP not established";
    const label=st.leaseHostname?" · Lease label: "+st.leaseHostname+" (unverified identity)":"";
    result.push([r.interface+" · "+st.mac,signal+(rates?" · "+rates:"")+neighbor+label]);
   }
  }
 }
 result.push(["Station address privacy","MAC addresses are displayed only in this authenticated device dashboard; do not publish screenshots with addresses."]);
 return result;
}
function vpnFacts(d) {
 const p=d.platform||{},k=p.kernelEvidence||{};
 return [["TUN character device",p.tunDevicePresent?"Present":"Not found"],["TUN misc device registration",k.tunRegistered===true?"Registered":k.tunRegistered===false?"Not registered":"Unknown"],["Kernel CONFIG_TUN",k.tunKernelConfig||"Not readable"],["TUN feasibility",k.tunAssessment||"Unverified"],
 ["WireGuard module",p.wireGuardModulePresent?"Observed":"Not observed"],["Kernel CONFIG_WIREGUARD",k.wireGuardKernelConfig||"Not readable"],["WireGuard feasibility",k.wireGuardAssessment||"Unverified"],["Kernel XFRM status",k.xfrmStatPresent?"Status endpoint present":"Not observed"],["VPN tools in PATH",hasArray(p.vpnExecutables).join(", ")||"None detected"],["VPN connection","Not configured or established by HGFW"]];
}
function renderPorts(d){
 const mount=$("lan-ports"); if(!mount)return;
 const items=hasArray(d.lanPorts);
 const cells=items.map(p=>{
  const card=el("article","lan-port");
  const header=el("div","lan-port-top");
  header.append(el("strong","",p.label),badge(p.carrier===true?"Link detected":p.carrier===false?"No carrier":"Unknown",p.carrier===true?"up":""));
  card.append(header,el("p","lan-port-device",p.interface+" / "+p.bridgeInterface),
   el("p","lan-port-caption",p.master?"Bridge member: "+p.master:"Bridge assignment unknown"));
  card.title=p.source||"Mapping requires board verification";
  return card;
 });
 mount.replaceChildren(...(cells.length?cells:[emptyState("No board-specific mapping","LAN jack labels are not inferred for unrecognized devices.","⇄")]));
}
function routeCatalog(){
 const full=location.hash.slice(1).split("/");
 const group=full[0], feature=full[1];
 if(!Object.prototype.hasOwnProperty.call(featureCatalog,group))return;
 renderCatalog(group,feature);
}
function renderCatalog(group,requested){
 const host=$("catalog-"+group);
 if(!host)return;
 const items=featureCatalog[group]||[];
 const selected=items.find(x=>x[0]===requested)||items[0];
 const index=el("div","catalog-index");
 index.setAttribute("role","group");index.setAttribute("aria-label","Choose "+group+" feature");
 for(const f of items){
  const button=el("button","catalog-entry"+(f===selected?" selected":""),f[1]);
  button.type="button";
  button.setAttribute("aria-pressed",String(f===selected));
  button.addEventListener("click",()=>{location.hash=group+"/"+f[0];});
  index.append(button);
 }
 const detail=el("article","catalog-detail");
 const heading=el("div","catalog-head");
 const title=el("h2","",selected[1]);
 const status=badge(selected[2],selected[2].startsWith("Live")?"up":"");
 heading.append(title,status);
 detail.append(heading,el("p","catalog-lede",selected[3]));
 const facts=latest?selected[4](latest.device):[];
 if(facts.length) {
  const dl=el("dl","fact-grid");
  for(const [key,value] of facts){
   const item=el("div","fact-entry");
   item.append(el("dt","",key),el("dd","",value==null?"Unknown":value));
   dl.append(item);
  }
  detail.append(dl);
 }else if(!latest)detail.append(el("p","catalog-help","Waiting for authenticated device status…"));
 else detail.append(el("div","unavailable-feature","This setting is not connected to an independent HGFW implementation. No apply, save or stock-firmware write operation is available."));
 detail.append(el("p","catalog-footnote","Information source: "+(selected[2].startsWith("Live")||selected[2].includes("Kernel")||selected[2].includes("Prerequisite")||selected[2].includes("driver probe")?"Linux kernel / HGFW diagnostics; optional native read-only helper.":"PLDT menu capability reference only.")+" The dashboard does not invoke proprietary /goform settings."));
 host.replaceChildren(index,detail);
}
function renderAllCatalogs(){
 for(const name of Object.keys(featureCatalog))renderCatalog(name,location.hash.slice(1).split("/")[0]===name?location.hash.split("/")[1]:undefined);
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
    renderPorts(data.device);
    renderStorage(data.device.disks);
    renderServices(services);
    renderAllCatalogs();
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
renderAllCatalogs();
setPage();
refresh();
setInterval(()=>{if(!document.hidden)refresh();},15000);
