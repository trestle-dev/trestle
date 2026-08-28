const databasePreview=document.querySelector("#database-preview");
const postgresConfiguration=document.querySelector("#postgres-configuration");
const databaseResult=document.querySelector("#database-result");
const databaseApply=document.querySelector("#database-apply");
function selectedDatabase(){return databasePreview.querySelector('[name="database-provider"]:checked').value}
const administratorFields=document.querySelector("#administrator-fields");
let databaseSelectable=false;
function syncDatabaseFields(){const firstRun=authForm.classList.contains("first-run");const pendingPostgres=firstRun&&databaseSelectable&&selectedDatabase()==="postgres";databasePreview.hidden=!firstRun;postgresConfiguration.hidden=!pendingPostgres;databaseApply.hidden=!pendingPostgres;administratorFields.hidden=pendingPostgres;authEmail.required=!pendingPostgres;authPassword.required=!pendingPostgres;if(!pendingPostgres)databaseResult.textContent=""}
new MutationObserver(syncDatabaseFields).observe(authForm,{attributes:true,attributeFilter:["class"]});
databasePreview.addEventListener("change",syncDatabaseFields);
databaseApply.addEventListener("click",async()=>{databaseResult.textContent="Testing PostgreSQL…";databaseApply.disabled=true;try{const result=await jsonRequest("/admin/v1/database/setup",{method:"POST",body:JSON.stringify({provider:"postgres",url:databasePreview.querySelector('[name="database-url"]').value})});databaseSelectable=false;databasePreview.disabled=true;databaseResult.textContent=`PostgreSQL ${result.version} is configured. Restart Trestle to create the administrator.`;administratorFields.hidden=true;postgresConfiguration.hidden=true}catch(error){databaseResult.textContent=error.message}finally{databaseApply.disabled=false}});
async function loadDatabaseSetup(){try{const result=await jsonRequest("/admin/v1/database/setup");databaseSelectable=Boolean(result.selectable);const radio=databasePreview.querySelector(`[value="${result.provider}"]`);if(radio)radio.checked=true;databasePreview.disabled=!databaseSelectable;syncDatabaseFields()}catch{}}
loadDatabaseSetup();syncDatabaseFields();
