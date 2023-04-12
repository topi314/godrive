let files = [];
let requests = [];

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

function uploadFile(method, file, name, description, filePrivate, errorID, progressID) {
    const data = new FormData();
    const json = {
        size: file ? file.size : null,
        description: description,
        private: filePrivate,
    };
    data.append("json", JSON.stringify(json));
    if (file) {
        data.append("file", file, name || file.name);
    } else {
        data.append("file", new Blob([""]), name);
    }

    const rq = new XMLHttpRequest();
    rq.responseType = "json";
    rq.addEventListener("load", () => {
        if (rq.status >= 200 && rq.status < 300) {
            window.location.reload();
        } else {
            setUploadError(errorID, rq);
        }
    });
    rq.upload.addEventListener("error", () => {
        setUploadError(errorID, rq);
    });
    rq.upload.addEventListener("progress", (e) => {
        if (e.lengthComputable) {
            const percent = Math.round((e.loaded / e.total) * 100);
            document.querySelector(progressID).style.width = `${percent}%`;
        }
    });
    let url = window.location.pathname;
    if (method === "PATCH") {
        if (!url.endsWith("/")) {
            url += "/";
        }
        url += file.name;
    }
    rq.open(method, url);
    rq.send(data);
    requests.push(rq);
}

function setUploadError(errorID, request) {
    document.querySelector(errorID).textContent = request.response ? request.response.message : request.statusText || "Unknown error";
}
