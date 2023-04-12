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
    for (let i = 0; i < files.length; i++) {
        uploadFile("POST",
            files[i],
            document.querySelector(`#file-${i}-name`).value,
            document.querySelector(`#file-${i}-description`).value,
            document.querySelector(`#file-${i}-private`).checked,
            `#upload-${i}-error`,
            `#upload-${i}-progress-bar`
        );
    }

});

document.querySelector("#upload-dialog").addEventListener("close", () => {
    for (const request of requests) {
        request.abort();
    }
    files = [];
    requests = [];
    document.querySelector("#upload-files").replaceChildren();
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
        
        <div id="upload-${i}-error" class="error"></div>
        <div id="upload-${i}-progress-bar" class="progress-bar"></div>
    </div>

    <div>
        <button id="close" class="icon-btn" onclick="this.parentElement.parentElement.remove() "></button>
    </div>`;
    return div;
}