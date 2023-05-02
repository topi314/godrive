register("#files", "change", (e) => {
    e.preventDefault();
    e.stopPropagation();
    files.splice(0, files.length, ...e.target.files);
    openUploadDialog();
});

register("#upload-cancel-btn", "click", () => {
    document.querySelector("#upload-dialog").close();
});

register("#upload-confirm-btn", "click", (e) => {
    e.preventDefault();
    e.stopPropagation();
    const uploadDir = document.querySelector("#upload-file-dir");
    uploadDir.disabled = true;
    const confirmBtn = document.querySelector("#upload-confirm-btn");
    confirmBtn.disabled = true;
    let done = 0;
    for (let i = 0; i < files.length; i++) {
        const fileName = document.querySelector(`#file-${i}-name`);
        const fileDescription = document.querySelector(`#file-${i}-description`);

        fileName.disabled = true;
        fileDescription.disabled = true;

        uploadFile("POST",
            uploadDir.value,
            files[i],
            undefined,
            fileName.value,
            fileDescription.value,
            () => {
                done++;
                if (done === files.length) {
                    window.location.reload();
                }
            },
            (xhr) => {
                setUploadError(`#upload-${i}-error`, xhr)
            },
            (e) => {
                document.querySelector(`#upload-${i}-progress-bar`).style.width = `${e.loaded / e.total * 100}%`;
            }
        );
    }
});

register("#upload-dialog", "close", () => {
    for (const request of requests) {
        request.abort();
    }
    files.splice(0, files.length);
    requests.splice(0, requests.length);
    document.querySelector("#upload-files").replaceChildren();
    document.querySelector("#upload-file-dir").disabled = false;
    document.querySelector("#upload-confirm-btn").disabled = false;
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
    
    <div class="upload-file-content">
        <div id="upload-file-fields">
            <label for="file-${i}-name">Name</label>
            <div><input type="text" id="file-${i}-name" value="${file.name}"></div>
    
            <label for="file-${i}-description">Description</label>
            <div><textarea id="file-${i}-description"></textarea></div>
        </div>
        <div id="upload-${i}-error" class="upload-error"></div>
        <div class="progress">
            <div id="upload-${i}-progress-bar" class="progress-bar"></div>
        </div>
    </div>

    <div>
        <button id="close" class="icon-btn" onclick="this.parentElement.parentElement.remove() "></button>
    </div>`;
    return div;
}