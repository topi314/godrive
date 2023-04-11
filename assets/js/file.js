
registerAll(".file-upload", "dragover", (e) => toggleUploadActive(e, true));

registerAll(".file-upload", "dragenter", (e) => toggleUploadActive(e, true));

registerAll(".file-upload", "dragleave", (e) => toggleUploadActive(e, false));

registerAll(".file-upload", "dragend", (e) => toggleUploadActive(e, false));

registerAll(".file-upload", "drop", (e) => {
    toggleUploadActive(e, false);
    files = e.dataTransfer.files;
    openUploadDialog();
});

registerAll(".file-more", "change", (e) => {
    e.preventDefault();
    e.stopPropagation();

    if (e.target.value === "edit") {
        openEditDialog(e.target.dataset);
    } else if (e.target.value === "delete") {
        if (!confirm("Are you sure you want to delete this file or folder?")) {
            e.target.value = "none";
            return;
        }
        const rq = new XMLHttpRequest();
        rq.responseType = "json";
        rq.addEventListener("load", () => {
            if (rq.status >= 200 && rq.status < 300) {
                window.location.reload();
            } else {
                alert(rq.response.message || rq.statusText);
            }
        })
        rq.open("DELETE", e.target.dataset.file);
        rq.send();
    } else if (e.target.value === "share") {
        document.querySelector("#share-dialog").showModal();
    }
    e.target.value = "none";
});
