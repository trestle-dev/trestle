const databasePreview=document.querySelector("#database-preview");
function syncDatabasePreview(){databasePreview.hidden=!authForm.classList.contains("first-run")}
new MutationObserver(syncDatabasePreview).observe(authForm,{attributes:true,attributeFilter:["class"]});
syncDatabasePreview();
