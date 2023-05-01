register("#file", "change", (e) => {
    e.preventDefault();
    e.stopPropagation();
    files = e.target.files;
    document.querySelector("#edit-file-new-name").value = files[0].name;
});

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
    let path = window.location.pathname;
    if (!path.endsWith("/")) {
        path += "/";
    }
    path += document.querySelector("#edit-file-name").value;

    const fileNewDir = document.querySelector("#edit-file-new-dir");
    const fileNewName = document.querySelector("#edit-file-new-name");
    const fileDescription = document.querySelector("#edit-file-description");
    const filePrivate = document.querySelector("#edit-file-private");

    fileNewDir.disabled = true;
    fileNewName.disabled = true;
    fileDescription.disabled = true;
    filePrivate.disabled = true;

    document.querySelector("#edit-upload").style.display = "none";
    document.querySelector("#edit-feedback").style.display = "flex";

    uploadFile("PATCH",
        path,
        file,
        fileNewDir.value,
        fileNewName.value,
        fileDescription.value,
        filePrivate.checked,
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
    document.querySelector("#edit-upload").style.display = "flex";
});


function openEditDialog(dataset) {
    document.querySelector("#edit-file-name").value = dataset.name;
    document.querySelector("#edit-file-new-dir").value = window.location.pathname;
    document.querySelector("#edit-file-new-name").value = dataset.name;
    document.querySelector("#edit-file-description").value = dataset.description;
    document.querySelector("#edit-file-private").checked = dataset.private === "true";
    document.querySelector("#edit-dialog").showModal();
}