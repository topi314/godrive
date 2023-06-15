
export function uploadFile(method, path, file, dir, name, description, doneCallback, errorCallback, progressCallback) {
	const data = new FormData();
	const json = {
		size: file ? file.size : null,
		description: description,
	};
	if (dir) {
		json.dir = dir;
	}
	data.append("json", JSON.stringify(json));
	if (file) {
		data.append("file", file, name || file.name);
	} else {
		data.append("file", new Blob([""]), name);
	}

	const xhr = new XMLHttpRequest();
	xhr.responseType = "json";
	xhr.addEventListener("load", () => {
		if (xhr.status >= 200 && xhr.status < 300) {
			doneCallback(xhr);
		} else {
			errorCallback(xhr);
		}
	});
	xhr.upload.addEventListener("error", () => {
		errorCallback(xhr);
	});
	xhr.upload.addEventListener("progress", (e) => {
		if (e.lengthComputable) {
			progressCallback(e);
		}
	});
	xhr.open(method, path);
	xhr.send(data);
	return xhr;
}
