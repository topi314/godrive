registerAll(".file-upload", "dragover", (e) => toggleUploadActive(e, true));

registerAll(".file-upload", "dragenter", (e) => toggleUploadActive(e, true));

registerAll(".file-upload", "dragleave", (e) => toggleUploadActive(e, false));

registerAll(".file-upload", "dragend", (e) => toggleUploadActive(e, false));

registerAll(".file-upload", "drop", (e) => {
    toggleUploadActive(e, false);
    files.splice(0, files.length, ...e.dataTransfer.files);
    openUploadDialog();
});

registerAll(".file-more", "change", (e) => {
    e.preventDefault();
    e.stopPropagation();

    switch (e.target.value) {
        case "download":
            downloadFiles();
            break;
        case "edit":
            openEditDialog(e.target.dataset);
            break;

        case "move":
            openMoveDialog();
            break;

        case "delete":
            openDeleteDialog(e);
            break;

        case "share":
            document.querySelector("#share-dialog").showModal();
            break;
    }
    e.target.value = "none";
});

register("#files-select", "click", (e) => {
    if (!e.target.checked) {
        selectedFiles.splice(0, selectedFiles.length);
    }
    for (const child of document.querySelector("#file-list").children) {
        const fileSelect = child.querySelector(".file-select");
        if (!fileSelect) {
            continue;
        }
        if (!e.target.checked) {
            fileSelect.checked = false;
            continue;
        }
        fileSelect.checked = true;
        selectedFiles.push(fileSelect.dataset.name);
    }
    document.querySelector("#files-more").disabled = selectedFiles.length === 0
})

registerAll(".file-select", "click", (e) => {
    if (e.target.checked) {
        selectedFiles.push(e.target.dataset.name);
        if (selectedFiles.length === document.querySelector("#file-list").children.length - 1) {
            document.querySelector("#files-select").checked = true;
        }
    } else {
        selectedFiles.splice(selectedFiles.indexOf(e.target.dataset.name), 1);
        document.querySelector("#files-select").checked = false;
    }
    document.querySelector("#files-more").disabled = selectedFiles.length === 0
});