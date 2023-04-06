let files = [];

document.querySelector("#file-upload").addEventListener("dragover", (e) => {
    e.preventDefault();
    e.stopPropagation();

    document.querySelector("#file-upload").classList.add("active");
});

document.querySelector("#file-upload").addEventListener("dragenter", (e) => {
    e.preventDefault();
    e.stopPropagation();

    document.querySelector("#file-upload").classList.add("active");
});

document.querySelector("#file-upload").addEventListener("dragleave", (e) => {
    e.preventDefault();
    e.stopPropagation();

    document.querySelector("#file-upload").classList.remove("active");
});

document.querySelector("#file-upload").addEventListener("drop", (e) => {
    e.preventDefault();
    e.stopPropagation();

    document.querySelector("#file-upload").classList.remove("active");
    files = e.dataTransfer.files;
    openUploadPopup();
});

document.querySelector("#file").addEventListener("change", (e) => {
    e.preventDefault();
    e.stopPropagation();
    files = e.target.files;
    openUploadPopup();
});

document.querySelector("#cancel-btn").addEventListener("click", (e) => {
    document.querySelector("#upload-popup").close();
});

document.querySelector("#upload-btn").addEventListener("click", (e) => {
    e.preventDefault();
    e.stopPropagation();
    uploadFiles(files);
});


document.querySelector("#upload-popup").addEventListener("close", () => {
    files = [];
    document.querySelector("#upload-popup-files").replaceChildren();
    document.querySelector("#upload-popup-error").textContent = "";
    document.querySelector("#upload-popup-progress-bar").style.width = "0";
    document.querySelector("#upload-files").style.display = "flex";
    document.querySelector("#upload-files-feedback").style.display = "none";
});

function openUploadPopup() {
    const main = document.querySelector("#upload-popup-files");
    for (let i = 0; i < files.length; i++) {
        main.appendChild(getDialogFileElement(i, files[i]));
    }
    document.querySelector("#upload-popup").showModal();
}

function getDialogFileElement(i, file) {
    const div = document.createElement("div");
    div.classList.add("upload-file");
    div.innerHTML = `
    <div>
        <span class="icon icon-large file-icon"></span>
    </div>
    
    <div class="upload-file-fields">
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

function uploadFiles(files) {
    const data = new FormData();
    const json = {
        dir: document.querySelector("#upload-file-dir").value,
        files: []
    }
    for (let i = 0; i < files.length; i++) {
        json.files.push({
            dir: window.location.pathname,
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
    document.querySelector("#upload-files-feedback").style.display = "flex";

    const rq = new XMLHttpRequest();
    rq.responseType = "json";
    rq.addEventListener("load", () => {
        console.log("load", rq.status);
        if (rq.status === 200) {
            window.location.reload();
        } else {
            setUploadError(rq.response.message || rq.statusText);
        }
    });
    rq.upload.addEventListener("error", () => {
        console.log("error", rq);

        if (rq.response) {
            setUploadError(rq.response.message || rq.statusText);
            return;
        }
        setUploadError(rq.statusText);
    });
    rq.upload.addEventListener("progress", (e) => {
        console.log(e);
        if (e.lengthComputable) {
            const percent = Math.round((e.loaded / e.total) * 100);
            document.querySelector("#upload-popup-progress-bar").style.width = `${percent}%`;
        }
    });
    rq.open("POST", "/api/files");
    rq.send(data);
}

function setUploadError(message) {
    document.querySelector("#upload-popup-error").textContent = message || "Unknown error";
}
