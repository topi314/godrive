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
})

function openUploadPopup() {
    console.log("openUploadPopup", files);
    const main = document.querySelector("#upload-popup-files");
    main.innerHTML = "";
    for (let i = 0; i < files.length; i++) {
        const file = files[i];
        const label = document.createElement("label");
        label.innerText = file.name;
        label.htmlFor = `file-${i}-name`;
        main.appendChild(label);

        const name = document.createElement("input");
        name.type = "text";
        name.id = `file-${i}-name`;
        name.value = file.name;
        name.placeholder = "Name";

        main.appendChild(name);
    }
    document.querySelector("#upload-popup").showModal();
}

function uploadFiles(files) {
    const data = new FormData();
    const json = [];
    for (let [i, file] of files.entries()) {
        json.push({
            dir: window.location.pathname,
            size: file.size,
            description: document.querySelector(`#file-${i}-description`).value,
            private: document.querySelector(`#file-${i}-private`).checked,
        });
        data.append("file", file, file.name);
    }
    data.append("json", JSON.stringify(json));

    for (let [i, file] of files.entries()) {
        data.append(`files[${i}]`, file, document.querySelector(`#file-${i}-name`).value || file.name);
    }

    const rq = new XMLHttpRequest();
    rq.open("POST", "/api/files");
    rq.onload = () => {
        if (rq.status === 200) {
            window.location.reload();
        } else {
            console.error("error uploading file:", rq);
            showErrorPopup(rq.statusText);
        }
    }
    rq.onerror = () => {
        console.error("error uploading file:", rq);
        showErrorPopup(rq.statusText);
    }
    rq.onprogress = (e) => {
        if (e.lengthComputable) {
            const percent = Math.round((e.loaded / e.total) * 100);
            document.querySelector("#upload-progress").style.width = `${percent}%`;
        }
    }
    rq.send(data);

}

function showErrorPopup(message) {
    const popup = document.getElementById("error-popup");
    popup.style.display = "block";
    popup.innerText = message || "Something went wrong.";
    setTimeout(() => popup.style.display = "none", 5000);
}