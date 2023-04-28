register("#edit-cancel-btn", "click", () => {
    document.querySelector("#edit-dialog").close();
});

register("#edit-confirm-btn", "click", (e) => {
    e.preventDefault();
    e.stopPropagation();
    let file;
    if (files && files.length > 0) {
        file = files[0];
    }
    uploadFile("PATCH",
        window.location.pathname,
        file,
        document.querySelector("#edit-file-new-dir").value,
        document.querySelector("#edit-file-new-name").value,
        document.querySelector("#edit-file-description").value,
        document.querySelector("#edit-file-private").checked,
        (xhr) => {
            window.location.reload();
        },
        (xhr) => {
            setUploadError("#edit-error", xhr)
        },
        (e) => {
            document.querySelector("#edit-progress-bar").style.width = `${e.loaded / e.total * 100}%`;
        }
    );
});

register("#edit-dialog", "close", () => {
    for (const request of requests) {
        request.abort();
    }
    files = [];
    requests = [];
    document.querySelector("#edit-error").textContent = "";
    document.querySelector("#edit-progress-bar").style.width = "0";
    document.querySelector("#edit-file").style.display = "flex";
    document.querySelector("#edit-feedback").style.display = "none";
});


function openEditDialog(dataset) {
    document.querySelector("#edit-file-name").value = dataset.name;
    document.querySelector("#edit-file-new-dir").value = window.location.pathname;
    document.querySelector("#edit-file-new-name").value = dataset.name;
    document.querySelector("#edit-file-description").value = dataset.description;
    document.querySelector("#edit-file-private").checked = dataset.private === "true";
    document.querySelector("#edit-dialog").showModal();
}