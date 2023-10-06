const selectedFiles = [];

export function getSelectedFiles() {
	const files = selectedFiles.slice();
	selectedFiles.splice(0, selectedFiles.length);
	return files;
}

export function onFilesSelect(e) {
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
	document.querySelector("#files-more").disabled = selectedFiles.length === 0
}

export function onFileSelect(e) {
	if (e.target.checked) {
		selectedFiles.push(e.target.dataset.name);
		if (selectedFiles.length === document.querySelector("#file-list").children.length - 1) {
			document.querySelector("#files-select").checked = true;
		}
	} else {
		selectedFiles.splice(selectedFiles.indexOf(e.target.dataset.name), 1);
		document.querySelector("#files-select").checked = false;
	}
	document.querySelector("#files-more").disabled = selectedFiles.length === 0
}

export function onDownloadFiles(e) {
	const path = e.target.dataset.file;
	if (path) {
		window.open(`${path}?dl=1`, "_blank");
		return;
	}
	window.open(`${window.location.href}?dl=1&files=${selectedFiles.join(",")}`, "_blank");
}

export default {
	getSelectedFiles,
	onFilesSelect,
	onFileSelect,
	onDownloadFiles,
}