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
    let html = "";
    for (let i = 0; i < e.target.files.length; i++) {
      const file = e.target.files[i];
      html += `<li id="upload-file-${i}" xmlns="http://www.w3.org/1999/html">
	<div class="upload-file">
		<div class="upload-file-icon">
			<span class="icon icon-large file-icon"></span>
		</div>
		<div class="upload-file-content">
			<label>Name:</label><input name="name-${i}" value="${file.name}"/>
			<label>Description:</label><textarea  name="description-${i}"></textarea>
			<label>Overwrite:</label><span><input id="overwrite-${i}" class="checkbox" type="checkbox" name="overwrite-${i}" value="true" checked/><label for="overwrite-${i}" class="icon"></label></span>
		</div>
		<div class="upload-file-icon">
			<div class="icon-btn icon-remove" role="button" onclick="window.onUploadFileDelete(${i})"></div>
		</div>
	</div>
</li>`;
      document.getElementById("upload-files").innerHTML = html;
      document.getElementById("upload-button").disabled = false;
    }
  }
  function onFilesReset(event) {
    document.getElementById("upload-files").replaceChildren();
    document.getElementById("upload-button").disabled = true;
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
  var upload_default = {
    updateUploadProgress,
    onFilesChange,
    onFilesReset,
    onUploadFileDelete
  };

  // assets/js/main.js
  window.onFilesSelect = onFilesSelect;
  window.onFileSelect = onFileSelect;
  window.onDownloadFiles = onDownloadFiles;
  window.updateUploadProgress = updateUploadProgress;
  window.onFilesChange = onFilesChange;
  window.onFilesReset = onFilesReset;
  window.onUploadFileDelete = onUploadFileDelete;
  htmx.config.defaultErrorSwapStyle = "innerHTML";
  htmx.config.defaultErrorTarget = "mirror";
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
          size: file.size
        });
        data.set("json", JSON.stringify(files));
        data.append(`file-${i}`, file, params[`name-${i}`] || file.name);
      }
      return data;
    },
    onEvent: (name, event) => {
      if (name === "htmx:configRequest") {
        if (event.detail.parameters) {
          event.detail.path = event.detail.parameters.dir;
        }
      }
    }
  });
  htmx.defineExtension("edit-file", {
    encodeParameters: (xhr, params, element) => {
      const data = new FormData();
      const file = params.files[0];
      const json = {
        dir: params["dir"],
        name: params["name"] || file.name,
        description: params["description"]
      };
      if (file) {
        json.size = file.size;
      }
      data.append("json", JSON.stringify(json));
      if (file) {
        data.append("file", file, json.name);
      }
      return data;
    }
  });
  htmx.defineExtension("move-files", {
    onEvent: (name, event) => {
      if (name === "htmx:configRequest") {
        event.detail.useUrlParams = true;
        event.detail.parameters["files"] = getSelectedFiles().join(",");
      }
    }
  });
  htmx.defineExtension("delete-files", {
    onEvent: (name, event) => {
      if (name === "htmx:configRequest") {
        event.detail.useUrlParams = true;
        event.detail.parameters["files"] = getSelectedFiles().join(",");
      }
    }
  });
  htmx.defineExtension("destination-header", {
    encodeParameters: (xhr, params, element) => {
      return void 0;
    },
    onEvent: (name, event) => {
      if (name === "htmx:configRequest") {
        if (event.detail.parameters) {
          event.detail.headers["Destination"] = event.detail.parameters.dir;
        }
      }
    }
  });
  htmx.defineExtension("accept-html", {
    onEvent: (name, event) => {
      if (name === "htmx:configRequest") {
        event.detail.headers["Accept"] = "text/html";
      }
    }
  });
})();
