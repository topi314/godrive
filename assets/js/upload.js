export function updateUploadProgress(e) {
	document.getElementById("upload-progress").value = Math.min(e.detail.loaded / e.detail.total * 100, 100);
}

export function onFilesChange(e) {
	document.getElementById("upload-button").disabled = false;
}

export function onUploadFileDelete(i) {
	const dt = new DataTransfer();
	const files = document.getElementById("files");
	for(let j = 0; j < files.length; j++) {
		if(j !== i) {
			dt.items.add(files.files[j]);
		}
	}
	files.files = dt.files;
	document.getElementById("upload-file-" + i).remove();
}

export function onRemovePermissions(e) {
	e.target.parentElement.parentElement.remove();
}

export function stopBubble(e) {
	e.stopPropagation();
}

export default {
	updateUploadProgress,
	onFilesChange,
	onUploadFileDelete,
	onRemovePermissions,
	stopBubble
}
