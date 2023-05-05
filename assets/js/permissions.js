register("#save-permissions", "click", () => {
    const permissionList = document.querySelector("#permission-list");
    const permissions = [];
    for (const permission of permissionList.children) {
        if (permission.classList.contains("table-list-header")) {
            continue;
        }
        permissions.push({
            path: permission.querySelector(".permission-path").value,
            permissions: Array.from(permission.querySelectorAll(".permission-perms option:checked")).map(o => o.value),
            object_type: permission.querySelector(".permission-object-type").value,
            object: permission.querySelector(".permission-object").value,
        });
    }
    const rq = new XMLHttpRequest();
    rq.onload = () => {
        if (rq.status === 204) {
            // window.location.reload();
        } else if (rq.status === 200) {
            alert(rq.responseText);
        } else {
            alert(rq.responseText);
        }
    }
    rq.open("PUT", "/settings/permissions");
    rq.setRequestHeader("Content-Type", "application/json");
    rq.send(JSON.stringify(permissions));
});

register("#new-permission", "click", () => {
    const div = document.createElement("div");
    div.classList.add("table-list-entry");
    div.innerHTML = `
    <div><input class="permission-path" type="text" placeholder="path"></div>
    <div>
        <select class="permission-perms" multiple>
            <option value="read">Read</option>
            <option value="write">Write</option>
            <option value="create">Create</option>
            <option value="delete">Delete</option>
            <option value="share">Share</option>
        </select>
    </div>
    <div>
        <select class="permission-object-type">
            <option value="user">User</option>
            <option value="group">Group</option>
        </select>
    </div>
    <div>
        <input class="permission-object" type="text" placeholder="User/Group">
    </div>
    `;

    document.querySelector("#permission-list").appendChild(div);
});