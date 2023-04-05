document.querySelector("#upload-dialog-btn").addEventListener("click", async () => {
    document.querySelector("#upload-dialog").showModal();
});

document.querySelector("#upload-dialog-close").addEventListener("click", () => {
    document.querySelector("#upload-dialog").close();
});

document.querySelector("#upload-file").addEventListener("change", () => {
    const name = document.querySelector("#upload-name")
    if (!name.value) {
        name.value = document.querySelector("#upload-file").files[0].name;
    }
});

document.querySelector("#upload-btn").addEventListener("click", async () => {
    const file = document.querySelector("#upload-file").files[0];
    const data = new FormData();
    data.append("json", JSON.stringify({
        dir: document.querySelector("#upload-dir").value,
        size: file.size,
        description: document.querySelector("#upload-description").value,
        private: document.querySelector("#upload-private").checked,
    }));
    data.append("file", file, document.querySelector("#upload-name").value || file.name);

    const btn = document.querySelector("#upload-btn");
    btn.disabled = true;
    btn.classList.add("loading");

    const response = await fetch("/api/files", {
        method: "POST",
        body: data,
    });
    let body = await response.text();
    try {
        body = JSON.parse(body);
    } catch (e) {
        body = {message: body};
    }

    btn.classList.remove("loading");
    btn.disabled = false;

    document.querySelector("#upload-dialog").close();
    if (!response.ok) {
        console.error("error uploading file:", response);
        showErrorPopup(body.message || response.statusText);
    }
});

function showErrorPopup(message) {
    const popup = document.getElementById("error-popup");
    popup.style.display = "block";
    popup.innerText = message || "Something went wrong.";
    setTimeout(() => popup.style.display = "none", 5000);
}