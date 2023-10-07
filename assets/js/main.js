import "./theme.js";
import {getSelectedFiles, onFilesSelect, onFileSelect, onDownloadFiles} from "./files.js";
import {onFilesChange, onFilesReset, onUploadFileDelete, updateUploadProgress} from "./upload";

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
				event.detail.path = event.detail.parameters.dir
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
			description: params["description"],
		};

		if (file) {
			json.size = file.size
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
		return undefined;
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
