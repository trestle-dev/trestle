async function renderRecords(collection){
  document.getElementById("overview-content").hidden=true;document.getElementById("retry").hidden=true;
  const host=document.getElementById("view-content");
  document.getElementById("view-title").textContent=collection;
  host.className="record-view";
  host.innerHTML=`<div class="record-toolbar"><div><p class="eyebrow">Collection records</p><h2>${escapeHTML(collection)}</h2></div><button type="button" data-new-record>New record</button></div><form class="query-bar"><label>Filter expression<input name="filter" placeholder='status = "open" && score >= 3'></label><label>Sort<input name="sort" placeholder="-updatedAt"></label><button>Apply</button><button type="button" data-copy-query>Copy request</button></form><p class="view-error" role="alert"></p><div class="record-list">Loading records…</div><dialog class="record-dialog"><form method="dialog"><h2>Record values</h2><p>Enter a JSON object using the collection's field names.</p><textarea name="values" rows="12">{}</textarea><p class="dialog-error" role="alert"></p><div><button value="cancel">Cancel</button><button value="save">Save</button></div></form></dialog>`;
  const dialog=host.querySelector("dialog");
  host.querySelector("[data-new-record]").addEventListener("click",()=>{dialog.dataset.id="";dialog.dataset.version="";dialog.querySelector("textarea").value="{}";dialog.showModal()});
  dialog.addEventListener("close",async()=>{if(dialog.returnValue!=="save")return;try{const values=JSON.parse(dialog.querySelector("textarea").value);const id=dialog.dataset.id;const options={method:id?"PATCH":"POST",headers:{"X-Trestle-CSRF":csrfToken},body:JSON.stringify({values})};if(id)options.headers["If-Match"]=`"${dialog.dataset.version}"`;await jsonRequest(`/admin/v1/data/${encodeURIComponent(collection)}/records${id?`/${encodeURIComponent(id)}`:""}`,options);await loadRecords(collection,host)}catch(error){dialog.querySelector(".dialog-error").textContent=error.message;dialog.showModal()}});
  const queryForm=host.querySelector(".query-bar");const saved=JSON.parse(localStorage.getItem(`trestle-query:${collection}`)||"{}");queryForm.elements.filter.value=saved.filter||"";queryForm.elements.sort.value=saved.sort||"";queryForm.addEventListener("submit",event=>{event.preventDefault();localStorage.setItem(`trestle-query:${collection}`,JSON.stringify({filter:queryForm.elements.filter.value,sort:queryForm.elements.sort.value}));loadRecords(collection,host)});host.querySelector("[data-copy-query]").addEventListener("click",()=>navigator.clipboard.writeText(location.origin+recordListURL(collection,host)));
  await loadRecords(collection,host);
}

async function loadRecords(collection,host){
  try{
    const response=await jsonRequest(recordListURL(collection,host));
    const list=host.querySelector(".record-list");
    if(!response.items.length){list.innerHTML='<section class="empty"><h2>No records yet</h2><p>Create the first record with the button above.</p></section>';return}
    list.innerHTML='<div class="table-scroll"><table class="collection-table"><thead><tr><th>ID</th><th>Values</th><th>Updated</th><th></th></tr></thead><tbody>'+response.items.map(item=>`<tr><td><code>${escapeHTML(item.id)}</code></td><td><pre>${escapeHTML(JSON.stringify(item.values,null,2))}</pre></td><td>${new Date(item.updatedAt).toLocaleString()}</td><td><button data-edit="${escapeHTML(item.id)}">Edit</button> <button data-remove="${escapeHTML(item.id)}">Delete</button></td></tr>`).join("")+'</tbody></table></div>';
    list.querySelectorAll("[data-edit]").forEach(button=>button.addEventListener("click",()=>{const record=response.items.find(item=>item.id===button.dataset.edit);const dialog=host.querySelector("dialog");dialog.dataset.id=record.id;dialog.dataset.version=record.version;dialog.querySelector("textarea").value=JSON.stringify(record.values,null,2);dialog.showModal()}));
    list.querySelectorAll("[data-remove]").forEach(button=>button.addEventListener("click",async()=>{const record=response.items.find(item=>item.id===button.dataset.remove);if(!confirm(`Delete ${record.id}?`))return;try{await jsonRequest(`/admin/v1/data/${encodeURIComponent(collection)}/records/${encodeURIComponent(record.id)}`,{method:"DELETE",headers:{"X-Trestle-CSRF":csrfToken,"If-Match":`"${record.version}"`}});await loadRecords(collection,host)}catch(error){host.querySelector(".view-error").textContent=error.message}}));
  }catch(error){host.querySelector(".view-error").textContent=error.message}
}

function recordListURL(collection,host){const form=host.querySelector(".query-bar");const params=new URLSearchParams({limit:"100"});if(form?.elements.filter.value)params.set("filter",form.elements.filter.value);if(form?.elements.sort.value)params.set("sort",form.elements.sort.value);return `/admin/v1/data/${encodeURIComponent(collection)}/records?${params}`}

function escapeHTML(value){return String(value).replace(/[&<>"']/g,char=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[char]))}
