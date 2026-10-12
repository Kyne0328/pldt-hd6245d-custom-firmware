"use strict";
const byId = (id) => document.getElementById(id);
function text(id, value) { byId(id).textContent = value; }
function row(parent, name, detail, status) {
  const item = document.createElement("div"); item.className = "entry";
  const info = document.createElement("div");
  const title = document.createElement("strong"); title.textContent = name;
  const subtitle = document.createElement("small"); subtitle.textContent = detail;
  const state = document.createElement("span"); state.className = "state"; state.textContent = status;
  info.append(title,subtitle); item.append(info,state); parent.append(item);
}
function empty(parent, message) {
  const p = document.createElement("p");p.className="empty";p.textContent=message;parent.append(p);
}
async function refresh() {
  const alert = byId("alert");
  try {
    const response = await fetch("/api/status", {credentials:"same-origin", cache:"no-store"});
    if (response.status === 401) {window.location.assign("/login");return;}
    if (!response.ok) throw new Error("HTTP " + response.status);
    const data = await response.json();
    text("model",data.device.model || "Unknown");
    text("ram",Math.round((data.device.memAvailableKB || 0)/1024) + " MiB");
    text("ifcount",String(data.device.interfaces.length));
    text("usbcount",String(data.device.disks.length));
    text("updated","Updated " + new Date().toLocaleTimeString());
    const interfaces=byId("interface-list"); interfaces.replaceChildren();
    for(const i of data.device.interfaces) row(interfaces,i.name,(i.addresses || []).join(" · ") || "No assigned IP address",i.flags);
    if(!data.device.interfaces.length) empty(interfaces,"No network interfaces reported.");
    const disks=byId("disk-list"); disks.replaceChildren();
    for(const d of data.device.disks) row(disks,d.device,d.mountpoint || Math.round(d.bytes/1024/1024) + " MiB · not mounted",d.transport);
    if(!data.device.disks.length) empty(disks,"No removable USB storage detected.");
    const services=byId("service-list"); services.replaceChildren();
    for(const s of data.services)row(services,s.name,"Restarts: "+s.restarts+(s.lastError ? " · "+s.lastError : ""),s.state);
    if(!data.services.length) empty(services,"No services configured. Runtime supervisor is idle.");
    alert.textContent="Live hardware observations. All configuration changes remain disabled.";
    alert.className="notice ok";
  } catch (err) {
    alert.textContent="Unable to retrieve device status: " + err.message;
    alert.className="notice error";
  }
}
byId("logout").addEventListener("click", async ()=>{
  try {
    const csrf=await (await fetch("/api/csrf",{cache:"no-store"})).json();
    const r=await fetch("/logout",{method:"POST",headers:{"X-CSRF-Token":csrf.csrf}});
    if(!r.ok) throw new Error("Sign-out failed");
    window.location.assign("/login");
  } catch(e){byId("alert").textContent=e.message;byId("alert").className="notice error";}
});
refresh();
setInterval(refresh,15000);
