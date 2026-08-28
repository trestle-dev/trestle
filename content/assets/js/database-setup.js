const databasePreview=document.querySelector("#database-preview");
const postgresConfiguration=document.querySelector("#postgres-configuration");
const databaseResult=document.querySelector("#database-result");
const databaseApply=document.querySelector("#database-apply");
const databaseUrlInput=databasePreview.querySelector('[name="database-url"]');
function selectedDatabase(){return databasePreview.querySelector('[name="database-provider"]:checked').value}
const administratorFields=document.querySelector("#administrator-fields");
let databaseSelectable=false;
function syncDatabaseFields(){const firstRun=authForm.classList.contains("first-run");const pendingPostgres=firstRun&&databaseSelectable&&selectedDatabase()==="postgres";databasePreview.hidden=!firstRun;postgresConfiguration.hidden=!pendingPostgres;databaseApply.hidden=!pendingPostgres;administratorFields.hidden=pendingPostgres;authEmail.required=!pendingPostgres;authPassword.required=!pendingPostgres;if(!pendingPostgres){databaseResult.textContent="";databaseResult.className=""}else{syncDatabaseApply()}}function syncDatabaseApply(){databaseApply.disabled=!databaseUrlInput.value.trim()}
new MutationObserver(syncDatabaseFields).observe(authForm,{attributes:true,attributeFilter:["class"]});
databaseUrlInput.addEventListener("input",syncDatabaseApply);
databasePreview.addEventListener("change",syncDatabaseFields);
databaseApply.addEventListener("click",async()=>{databaseResult.textContent="Testing PostgreSQL…";databaseResult.className="";databaseApply.disabled=true;try{const result=await jsonRequest("/admin/v1/database/setup",{method:"POST",body:JSON.stringify({provider:"postgres",url:databaseUrlInput.value})});databaseSelectable=false;databasePreview.disabled=true;databaseResult.className="restart-notice";databaseResult.innerHTML=`<strong>Restart required</strong><span>PostgreSQL ${escapeHTML(result.version)} is configured. Stop and start Trestle, then reload this page to create the administrator account.</span>`;administratorFields.hidden=true;postgresConfiguration.hidden=true}catch(error){databaseResult.textContent=error.message;databaseResult.className=""}finally{syncDatabaseApply()}});
async function loadDatabaseSetup(){try{const result=await jsonRequest("/admin/v1/database/setup");databaseSelectable=Boolean(result.selectable);const radio=databasePreview.querySelector(`[value="${result.provider}"]`);if(radio)radio.checked=true;databasePreview.disabled=!databaseSelectable;syncDatabaseFields()}catch{}}
loadDatabaseSetup();syncDatabaseFields();
