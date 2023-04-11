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
