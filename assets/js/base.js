const files = [];
const requests = [];

function registerAll(query, event, callback) {
    const elements = document.querySelectorAll(query);
    if (!elements) {
        return;
    }
    elements.forEach(element => {
        element.addEventListener(event, callback);
    });
}

function register(query, event, callback) {
    const element = document.querySelector(query);
    if (!element) {
        return;
    }
    element.addEventListener(event, callback);
}

function toggleUploadActive(e, active) {
    e.preventDefault();
    e.stopPropagation();
    e.target.classList.toggle("active", active);
}

function uploadFile(method, path, file, dir, name, description, doneCallback, errorCallback, progressCallback) {
    const data = new FormData();
    const json = {
        size: file ? file.size : null,
        description: description,
    };
    if (dir) {
        json.dir = dir;
    }
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
            doneCallback(rq);
        } else {
            errorCallback(rq);
        }
    });
    rq.upload.addEventListener("error", () => {
        errorCallback(rq);
    });
    rq.upload.addEventListener("progress", (e) => {
        if (e.lengthComputable) {
            progressCallback(e);
        }
    });
    rq.open(method, path);
    rq.send(data);
    requests.push(rq);
}

function setUploadError(errorID, request) {
    document.querySelector(errorID).textContent = request.response ? request.response.message : request.statusText || "Unknown error";
}
