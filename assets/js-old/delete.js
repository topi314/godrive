
function openDeleteDialog(e) {
    if (!confirm("Are you sure you want to delete this file or folder?")) {
        return;
    }
    const rq = new XMLHttpRequest();
    rq.responseType = "json";
    rq.addEventListener("load", () => {
        if (rq.status === 204) {
            window.location.reload();
        } else if (rq.status === 200) {
            alert(rq.response.message || rq.statusText);
        } else {
            alert(rq.response.message || rq.statusText);
        }
    })
    let body;
    if (e.target.id === "files-more") {
        rq.open("DELETE", window.location.pathname);
        rq.setRequestHeader("Content-Type", "application/json");
        body = JSON.stringify(selectedFiles);
    } else {
        rq.open("DELETE", e.target.dataset.file);
    }
    rq.send(body);
}