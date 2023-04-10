let files = [];
let rq = null;

function registerAll(query, event, callback) {
    document.querySelectorAll(query).forEach(element => {
        element.addEventListener(event, callback);
    });
}

function toggleUploadActive(e, active) {
    e.preventDefault();
    e.stopPropagation();
    e.target.classList.toggle("active", active);
}

registerAll(".file-upload", "dragover", (e) => toggleUploadActive(e, true));

registerAll(".file-upload", "dragenter", (e) => toggleUploadActive(e, true));

registerAll(".file-upload", "dragleave", (e) => toggleUploadActive(e, false));

registerAll(".file-upload", "dragend", (e) => toggleUploadActive(e, false));

registerAll(".file-upload", "drop", (e) => {
    toggleUploadActive(e, false);
    files = e.dataTransfer.files;
    openUploadDialog();
});

document.querySelector("#files").addEventListener("change", (e) => {
    e.preventDefault();
    e.stopPropagation();
    files = e.target.files;
    openUploadDialog();
});

document.querySelector("#upload-cancel-btn").addEventListener("click", () => {
    document.querySelector("#upload-dialog").close();
});

document.querySelector("#upload-confirm-btn").addEventListener("click", (e) => {
    e.preventDefault();
    e.stopPropagation();
    uploadFiles(files);
});

document.querySelector("#upload-dialog").addEventListener("close", () => {
    if (rq) {
        rq.abort();
    }
    files = [];
    document.querySelector("#upload-files").replaceChildren();
    document.querySelector("#upload-error").textContent = "";
    document.querySelector("#upload-progress-bar").style.width = "0";
    document.querySelector("#upload-files").style.display = "flex";
    document.querySelector("#upload-feedback").style.display = "none";
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
    uploadFile(file);
});

document.querySelector("#edit-dialog").addEventListener("close", () => {
    document.querySelector("#edit-error").textContent = "";
    document.querySelector("#edit-progress-bar").style.width = "0";
    document.querySelector("#edit-file").style.display = "flex";
    document.querySelector("#edit-feedback").style.display = "none";
});

function openUploadDialog() {
    const main = document.querySelector("#upload-files");
    for (let i = 0; i < files.length; i++) {
        main.appendChild(getDialogFileElement(i, files[i]));
    }
    document.querySelector("#upload-dialog").showModal();
}

function getDialogFileElement(i, file) {
    const div = document.createElement("div");
    div.classList.add("upload-file");
    div.innerHTML = `
    <div>
        <span class="icon icon-large file-icon"></span>
    </div>
    
    <div id="upload-file-fields">
        <label for="file-${i}-name">Name</label>
        <div><input type="text" id="file-${i}-name" value="${file.name}"></div>

        <label for="file-${i}-description">Description</label>
        <div><textarea id="file-${i}-description"></textarea></div>

        <label for="file-${i}-private">Private</label>
        <div><input type="checkbox" id="file-${i}-private"></div>
    </div>

    <div>
        <button id="close" class="icon-btn" onclick="this.parentElement.parentElement.remove() "></button>
    </div>`;
    return div;
}

function openEditDialog(dataset) {
    document.querySelector("#edit-file-name").value = dataset.name;
    document.querySelector("#edit-file-new-name").value = dataset.name;
    document.querySelector("#edit-file-description").value = dataset.description;
    document.querySelector("#edit-file-private").checked = dataset.private === "true";
    document.querySelector("#edit-dialog").showModal();
}

function uploadFile(file) {
    const data = new FormData();
    const json = {
        size: file ? file.size : null,
        description: document.querySelector(`#edit-file-description`).value,
        private: document.querySelector(`#edit-file-private`).checked,
    };
    data.append("json", JSON.stringify(json));
    if (file) {
        data.append("file", file, document.querySelector(`#edit-file-new-name`).value || file.name);
    } else {
        data.append("file", new Blob([""]), document.querySelector(`#edit-file-new-name`).value);
    }

    document.querySelector("#edit-file").style.display = "none";
    document.querySelector("#edit-feedback").style.display = "flex";

    let url = window.location.pathname;
    if (!url.endsWith("/")) {
        url += "/";
    }
    upload("PATCH", url + document.querySelector("#edit-file-name").value, data, "#edit-error", "#edit-progress-bar");
}

function uploadFiles(files) {
    const data = new FormData();
    const json = [];
    for (let i = 0; i < files.length; i++) {
        json.push({
            size: files[i].size,
            description: document.querySelector(`#file-${i}-description`).value,
            private: document.querySelector(`#file-${i}-private`).checked,
        });
    }
    data.append("json", JSON.stringify(json));

    for (let i = 0; i < files.length; i++) {
        const file = files[i];
        data.append(`files[${i}]`, file, document.querySelector(`#file-${i}-name`).value || file.name);
    }

    document.querySelector("#upload-files").style.display = "none";
    document.querySelector("#upload-feedback").style.display = "flex";

    upload("POST", document.querySelector("#upload-file-dir").value, data, "#upload-error", "#upload-progress-bar");
}

function upload(method, url, data, errorID, progressID) {
    const rq = new XMLHttpRequest();
    rq.responseType = "json";
    rq.addEventListener("load", () => {
        if (rq.status >= 200 && rq.status < 300) {
            window.location.reload();
        } else {
            setUploadError(errorID, rq.response.message || rq.statusText);
            console.error("failed to upload:", rq.response.message || rq.statusText)
        }
    });
    rq.upload.addEventListener("error", () => {
        if (rq.response) {
            setUploadError(errorID, rq.response.message || rq.statusText);
            console.error("error uploading:", rq.response.message || rq.statusText)
            return;
        }
        setUploadError(errorID, rq.statusText);
        console.error("error uploading:", rq.statusText)
    });
    rq.upload.addEventListener("progress", (e) => {
        if (e.lengthComputable) {
            const percent = Math.round((e.loaded / e.total) * 100);
            document.querySelector(progressID).style.width = `${percent}%`;
        }
    });
    rq.open(method, url);
    rq.send(data);
}

function setUploadError(errorID, message) {
    document.querySelector(errorID).textContent = message || "Unknown error";
}
