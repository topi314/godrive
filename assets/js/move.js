function openMoveDialog() {
    document.querySelector("#move-files-dir").value = window.location.pathname;
    document.querySelector("#move-dialog").showModal();
}

register("#move-cancel-btn", "click", () => {
    document.querySelector("#move-dialog").close();
});

register("#move-confirm-btn", "click", (e) => {
    e.preventDefault();
    e.stopPropagation();

    const moveDir = document.querySelector("#move-files-dir");
    moveDir.disabled = true;
    const confirmBtn = document.querySelector("#move-confirm-btn");
    confirmBtn.disabled = true;
    document.querySelector("#move-feedback").style.display = "flex";

    const rq = new XMLHttpRequest();
    rq.responseType = "json";
    rq.addEventListener("load", () => {
        if (rq.status >= 200 && rq.status < 300) {
            window.location.reload();
        } else {
            setUploadError(`#move-error`, rq)
        }
    })
    rq.open("PUT", window.location.pathname);
    rq.setRequestHeader("Content-Type", "application/json");
    rq.setRequestHeader("Destination", moveDir.value);
    rq.send(JSON.stringify(selectedFiles));
});

register("#move-dialog", "close", () => {
    for (const request of requests) {
        request.abort();
    }
    selectedFiles.splice(0, selectedFiles.length);
    document.querySelector("#move-feedback").style.display = "none";
    document.querySelector("#move-confirm-btn").disabled = false;
    document.querySelector("#move-files-dir").disabled = false;
});

register("#files-select", "click", (e) => {
    if (!e.target.checked) {
        selectedFiles.splice(0, selectedFiles.length);
    }
    for (const child of document.querySelector("#file-list").children) {
        const fileSelect = child.querySelector(".file-select");
        if (!e.target.checked) {
            fileSelect.checked = false;
            continue;
        }
        fileSelect.checked = true;
        selectedFiles.push(fileSelect.dataset.name);
    }
    document.querySelector("#files-more").disabled = selectedFiles.length === 0
})

registerAll(".file-select", "click", (e) => {
    if (e.target.checked) {
        selectedFiles.push(e.target.dataset.name);
    } else {
        selectedFiles.splice(selectedFiles.indexOf(e.target.dataset.name), 1);
    }
    document.querySelector("#files-more").disabled = selectedFiles.length === 0
});