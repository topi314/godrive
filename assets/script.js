document.querySelector("#upload").addEventListener("click", async () => {
    const file = document.querySelector("#file").files[0];
    const data = new FormData();
    data.append("json", JSON.stringify({
        path: document.querySelector("#path").value,
        size: file.size,
        description: document.querySelector("#description").value,
        private: document.querySelector("#private").checked,
    }));
    data.append("file", file, document.querySelector("#name").value || file.name);

    const response = await fetch("/api/files", {
        method: "POST",
        body: data,
    });
    if (!response.ok) {
        console.error("error uploading file:", response);
        return;
    }
    document.querySelector("#upload-result").innerHTML = await response.text();
});