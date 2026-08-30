const databasePreview=document.querySelector("#database-preview");
const postgresConfiguration=document.querySelector("#postgres-configuration");
const databaseResult=document.querySelector("#database-result");
const databaseApply=document.querySelector("#database-apply");
const databaseUrlInput=databasePreview.querySelector('[name="database-url"]');
function selectedDatabase(){return databasePreview.querySelector('[name="database-provider"]:checked').value}
const administratorFields=document.querySelector("#administrator-fields");
let databaseSelectable=false;
function databaseSetupState(){return TrestleDatabaseSetup.computeState({mode:authForm.classList.contains("first-run")?"first-run":"sign-in",selectable:databaseSelectable,provider:selectedDatabase(),url:databaseUrlInput.value})}
function syncDatabaseFields(){const state=databaseSetupState();databasePreview.hidden=!state.previewVisible;postgresConfiguration.hidden=!state.postgresConfigVisible;databaseApply.hidden=!state.applyVisible;databaseApply.disabled=!state.applyEnabled;administratorFields.hidden=!state.adminFormVisible;authEmail.required=state.adminEmailRequired;authPassword.required=state.adminPasswordRequired;if(!state.postgresConfigVisible){databaseResult.textContent="";databaseResult.className=""}}
function syncDatabaseApply(){databaseApply.disabled=!databaseSetupState().applyEnabled}
new MutationObserver(syncDatabaseFields).observe(authForm,{attributes:true,attributeFilter:["class"]});
databaseUrlInput.addEventListener("input",syncDatabaseApply);
databasePreview.addEventListener("change",syncDatabaseFields);
databaseApply.addEventListener("click",async()=>{databaseResult.textContent="Testing PostgreSQL…";databaseResult.className="";databaseApply.disabled=true;try{const result=await jsonRequest("/admin/v1/database/setup",{method:"POST",body:JSON.stringify({provider:"postgres",url:databaseUrlInput.value})});databaseSelectable=false;databasePreview.disabled=true;const notice=TrestleDatabaseSetup.restartNotice(result.version);databaseResult.className="restart-notice";databaseResult.innerHTML=`<strong>${escapeHTML(notice.heading)}</strong><span>${escapeHTML(notice.body)}</span>`;administratorFields.hidden=true;postgresConfiguration.hidden=true}catch(error){databaseResult.textContent=error.message;databaseResult.className=""}finally{syncDatabaseApply()}});
async function loadDatabaseSetup(){try{const result=await jsonRequest("/admin/v1/database/setup");databaseSelectable=Boolean(result.selectable);const radio=databasePreview.querySelector(`[value="${result.provider}"]`);if(radio)radio.checked=true;databasePreview.disabled=!databaseSelectable;syncDatabaseFields()}catch{}}
loadDatabaseSetup();syncDatabaseFields();