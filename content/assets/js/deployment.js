async function renderDeploymentSummary(){
  const host=document.getElementById("view-content");
  try{
    const deployment=await jsonRequest("/admin/v1/deployment");
    const card=document.createElement("section");
    card.className="settings-card deployment-card";
    card.innerHTML=`<div class="record-toolbar"><div><p class="eyebrow">Deployment</p><h2>Runtime and release</h2></div><a class="button-link" href="/admin/v1/support-bundle">Download support bundle</a></div><div class="api-summary"><article><span>Version</span><strong>${escapeHTML(deployment.version.version)}</strong><small>${escapeHTML(deployment.version.commit)}</small></article><article><span>Platform</span><strong>${escapeHTML(deployment.version.os)}/${escapeHTML(deployment.version.arch)}</strong><small>${escapeHTML(deployment.version.go)}</small></article><article><span>Trusted proxies</span><strong>${deployment.trustedProxies.length}</strong><small>${deployment.trustedProxies.length?escapeHTML(deployment.trustedProxies.join(", ")):"Forwarded headers ignored"}</small></article></div><p class="dialog-hint">Listening on ${escapeHTML(deployment.listen)} with ${escapeHTML(deployment.storageBackend)} storage. Support bundles exclude secrets.</p>`;
    host.prepend(card);
  }catch(error){ host.querySelector(".view-error").textContent=error.message }
}
const settingsObserver=new MutationObserver(()=>{const host=document.getElementById("view-content");if(host.querySelector(".rule-form")&&!host.querySelector(".deployment-card"))renderDeploymentSummary()});
settingsObserver.observe(document.getElementById("view-content"),{childList:true});
