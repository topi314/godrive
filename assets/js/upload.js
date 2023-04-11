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