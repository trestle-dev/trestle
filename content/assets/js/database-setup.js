const databasePreview=document.querySelector("#database-preview");
const databaseURL=document.querySelector("#postgres-url");
const databaseResult=document.querySelector("#database-result");
const databaseApply=document.querySelector("#database-apply");
function selectedDatabase(){return databasePreview.querySelector('[name="database-provider"]:checked').value}
function syncDatabaseFields(){databasePreview.hidden=!authForm.classList.contains("first-run");databaseURL.hidden=selectedDatabase()!=="postgres"}
new MutationObserver(syncDatabaseFields).observe(authForm,{attributes:true,attributeFilter:["class"]});
databasePreview.addEventListener("change",syncDatabaseFields);
databaseApply.addEventListener("click",async()=>{databaseResult.textContent="Testing connection…";databaseApply.disabled=true;try{const result=await jsonRequest("/admin/v1/database/setup",{method:"POST",body:JSON.stringify({provider:selectedDatabase(),url:databasePreview.elements?.["database-url"]?.value||databasePreview.querySelector('[name="database-url"]').value})});databaseResult.textContent=result.restartRequired?`PostgreSQL ${result.version} accepted. Restart Trestle to continue setup.`:`${result.provider} is ready for setup.`;if(result.restartRequired)authForm.querySelector("#auth-submit").disabled=true}catch(error){databaseResult.textContent=error.message}finally{databaseApply.disabled=false}});
async function loadDatabaseSetup(){try{const result=await jsonRequest("/admin/v1/database/setup");const radio=databasePreview.querySelector(`[value="${result.provider}"]`);if(radio)radio.checked=true;databasePreview.disabled=!result.selectable;databaseApply.hidden=!result.selectable;syncDatabaseFields()}catch{}}
loadDatabaseSetup();syncDatabaseFields();
