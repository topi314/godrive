(() => {
  // assets/js/theme.js
  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", (event) => {
    updateFaviconStyle(event.matches);
    setTheme(event.matches ? "dark" : "light");
  });
  document.addEventListener("DOMContentLoaded", () => {
    const matches = window.matchMedia("(prefers-color-scheme: dark)").matches;
    updateFaviconStyle(matches);
    const theme = getCookie("theme") || (matches ? "dark" : "light");
    setTheme(theme);
  });
  document.querySelector("#theme").addEventListener("click", () => {
    const theme = getCookie("theme");
    setTheme(theme === "dark" ? "light" : "dark");
  });
  function updateFaviconStyle(matches) {
    const faviconElement = document.querySelector(`link[rel="icon"]`);
    if (matches) {
      faviconElement.href = "/assets/favicon.png";
      return;
    }
    faviconElement.href = "/assets/favicon-light.png";
  }
  function setTheme(theme) {
    setCookie("theme", theme);
    document.documentElement.setAttribute("data-theme", theme);
    document.documentElement.classList.replace(theme === "dark" ? "light" : "dark", theme);
  }
  function getCookie(name) {
    let matches = document.cookie.match(new RegExp(
      "(?:^|; )" + name.replace(/([.$?*|{}()\[\]\\\/+^])/g, "\\$1") + "=([^;]*)"
    ));
    return matches ? decodeURIComponent(matches[1]) : void 0;
  }
  function setCookie(name, value, options = {}) {
    options = {
      path: "/",
      sameSite: "strict",
      ...options
    };
    if (options.expires instanceof Date) {
      options.expires = options.expires.toUTCString();
    }
    let updatedCookie = encodeURIComponent(name) + "=" + encodeURIComponent(value);
    for (let optionKey in options) {
      updatedCookie += "; " + optionKey;
      let optionValue = options[optionKey];
      if (optionValue !== true) {
        updatedCookie += "=" + optionValue;
      }
    }
    document.cookie = updatedCookie;
  }

  // assets/js/htmx-files.js
  htmx.defineExtension("new-files", {
    onEvent: (name, event) => {
      if (name === "htmx:configRequest") {
        event.detail.path = event.detail.parameters.dir;
        event.detail.useUrlParams = true;
        event.detail.parameters = {
          dir: event.detail.parameters.dir,
          action: "new-files",
          files: event.detail.parameters.files.map((file) => file.name).join(",")
        };
      }
    }
  });
  htmx.defineExtension("new-permissions", {
    onEvent: (name, event) => {
      if (name === "htmx:configRequest") {
        event.detail.path = event.detail.parameters.dir;
        event.detail.useUrlParams = true;
        const file = event.detail.parameters.file;
        event.detail.parameters = {
          dir: event.detail.parameters.dir,
          action: "new-permissions",
          file,
          index: event.detail.parameters.index,
          object_type: event.detail.parameters[`object_type-${file}`],
          object_name: event.detail.parameters[`object_name-${file}`]
        };
      }
    }
  });
  htmx.defineExtension("upload-files", {
    encodeParameters: (xhr, params, element) => {
      const data = new FormData();
      const files = [];
      for (let i = 0; i < params.files.length; i++) {
        const file = params.files[i];
        files.push({
          name: params[`name-${i}`] || file.name,
          description: params[`description-${i}`],
          overwrite: params[`overwrite-${i}`] === "true",
          size: file.size,
          permissions: parsePermissions(i, params)
        });
        data.set("json", JSON.stringify(files));
        data.append(`file-${i}`, file, params[`name-${i}`] || file.name);
      }
      return data;
    },
    onEvent: (name, event) => {
      if (name === "htmx:configRequest") {
        event.detail.path = event.detail.parameters.dir;
      }
    }
  });
  htmx.defineExtension("edit-file", {
    encodeParameters: (xhr, params, element) => {
      const data = new FormData();
      const file = params.files[0];
      const json = {
        dir: params["dir"],
        name: params["name-0"] || file.name,
        description: params["description-0"],
        permissions: parsePermissions(0, params)
      };
      if (file) {
        json.size = file.size;
      }
      data.append("json", JSON.stringify(json));
      if (file) {
        data.append("file", file, json.name);
      }
      return data;
    },
    onEvent: (name, event) => {
      if (name === "htmx:configRequest") {
      }
    }
  });
  htmx.defineExtension("edit-folder", {
    encodeParameters: (xhr, params, element) => {
      return void 0;
    },
    onEvent: (name, event) => {
      if (name === "htmx:configRequest") {
        event.detail.headers["Destination"] = event.detail.parameters.dir;
      }
    }
  });
  function parsePermissions(file, params) {
    const permissions = [];
    for (const permission of Object.keys(params)) {
      if (!permission.startsWith(`permissions-${file}`)) {
        continue;
      }
      const values = permission.split("-");
      const index = values[2];
      if (permissions.findIndex((p) => p.index === index) !== -1) {
        continue;
      }
      permissions.push({
        index,
        object_type: +params[`permissions-${file}-${index}-object_type`],
        object: params[`permissions-${file}-${index}-object`],
        permissions: {
          create: +params[`permissions-${file}-${index}-create`],
          update: +params[`permissions-${file}-${index}-update`],
          delete: +params[`permissions-${file}-${index}-delete`],
          update_permissions: +params[`permissions-${file}-${index}-update_permissions`],
          read: +params[`permissions-${file}-${index}-read`]
        }
      });
    }
    return permissions;
  }

  // assets/js/files.js
  var selectedFiles = [];
  function getSelectedFiles() {
    const files = selectedFiles.slice();
    selectedFiles.splice(0, selectedFiles.length);
    document.querySelector("#files-more").classList.toggle("disabled", true);
    return files;
  }
  function onFilesSelect(e) {
    if (!e.target.checked) {
      selectedFiles.splice(0, selectedFiles.length);
    }
    for (const child of document.querySelector("#file-list").children) {
      const fileSelect = child.querySelector(".file-select");
      if (!fileSelect) {
        continue;
      }
      if (!e.target.checked) {
        fileSelect.checked = false;
        continue;
      }
      fileSelect.checked = true;
      selectedFiles.push(fileSelect.dataset.name);
    }
    document.querySelector("#files-more").classList.toggle("disabled", selectedFiles.length === 0);
  }
  function onFileSelect(e) {
    if (e.target.checked) {
      selectedFiles.push(e.target.dataset.name);
      if (selectedFiles.length === document.querySelector("#file-list").children.length - 1) {
        document.querySelector("#files-select").checked = true;
      }
    } else {
      selectedFiles.splice(selectedFiles.indexOf(e.target.dataset.name), 1);
      document.querySelector("#files-select").checked = false;
    }
    document.querySelector("#files-more").classList.toggle("disabled", selectedFiles.length === 0);
  }
  function onDownloadFiles(e) {
    const path = e.target.dataset.file;
    if (path) {
      window.open(`${path}?dl=1`, "_blank");
      return;
    }
    window.open(`${window.location.href}?dl=1&files=${selectedFiles.join(",")}`, "_blank");
  }
  var files_default = {
    getSelectedFiles,
    onFilesSelect,
    onFileSelect,
    onDownloadFiles
  };

  // assets/js/upload.js
  function updateUploadProgress(e) {
    document.getElementById("upload-progress").value = Math.min(e.detail.loaded / e.detail.total * 100, 100);
  }
  function onFilesChange(e) {
    document.getElementById("upload-button").disabled = false;
  }
  function onUploadFileDelete(i) {
    const dt = new DataTransfer();
    const files = document.getElementById("files");
    for (let j = 0; j < files.length; j++) {
      if (j !== i) {
        dt.items.add(files.files[j]);
      }
    }
    files.files = dt.files;
    document.getElementById("upload-file-" + i).remove();
  }
  function onRemovePermissions(e) {
    e.target.parentElement.parentElement.remove();
  }
  function stopBubble(e) {
    e.stopPropagation();
  }
  var upload_default = {
    updateUploadProgress,
    onFilesChange,
    onUploadFileDelete,
    onRemovePermissions,
    stopBubble
  };

  // assets/js/share.js
  async function copyShareLink() {
    const shareLink = document.getElementById("share-link");
    shareLink.select();
    shareLink.setSelectionRange(0, 99999);
    await navigator.clipboard.writeText(shareLink.value);
  }
  var share_default = {
    copyShareLink
  };

  // assets/js/main.js
  window.onFilesSelect = onFilesSelect;
  window.onFileSelect = onFileSelect;
  window.onDownloadFiles = onDownloadFiles;
  window.updateUploadProgress = updateUploadProgress;
  window.onFilesChange = onFilesChange;
  window.onUploadFileDelete = onUploadFileDelete;
  window.onRemovePermissions = onRemovePermissions;
  window.stopBubble = stopBubble;
  window.copyShareLink = copyShareLink;
  htmx.defineExtension("accept-html", {
    onEvent: (name, event) => {
      if (name === "htmx:configRequest") {
        event.detail.headers["Accept"] = "text/html";
      }
    }
  });
})();
