document.querySelector("#edit-cancel-btn").addEventListener("click", () => {
    document.querySelector("#edit-dialog").close();
});

document.querySelector("#edit-confirm-btn").addEventListener("click", (e) => {
    e.preventDefault();
    e.stopPropagation();
    let file;
    if (files && files.length > 0) {
        file = files[0];
    }
    uploadFile("PATCH",
        file,
        document.querySelector("#edit-file-new-name").value,
        document.querySelector("#edit-file-description").value,
        document.querySelector("#edit-file-private").checked,
        "#edit-error",
        "#edit-progress-bar"
    );
});

document.querySelector("#edit-dialog").addEventListener("close", () => {
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
    document.querySelector("#edit-file-new-name").value = dataset.name;
    document.querySelector("#edit-file-description").value = dataset.description;
    document.querySelector("#edit-file-private").checked = dataset.private === "true";
    document.querySelector("#edit-dialog").showModal();
}