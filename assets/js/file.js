registerAll(".file-upload", "dragover", (e) => toggleUploadActive(e, true));

registerAll(".file-upload", "dragenter", (e) => toggleUploadActive(e, true));

registerAll(".file-upload", "dragleave", (e) => toggleUploadActive(e, false));

registerAll(".file-upload", "dragend", (e) => toggleUploadActive(e, false));

registerAll(".file-upload", "drop", (e) => {
    toggleUploadActive(e, false);
    files.splice(0, files.length, ...e.dataTransfer.files);
    openUploadDialog();
});
