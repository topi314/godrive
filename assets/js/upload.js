export function updateUploadProgress(e) {
	document.getElementById("upload-progress").value = Math.min(e.detail.loaded / e.detail.total * 100, 100);
}

export function onFilesChange(e) {
	let html = "";
	for(let i = 0; i < e.target.files.length; i++) {
		const file = e.target.files[i];
		html += `<li id="upload-file-${i}" xmlns="http://www.w3.org/1999/html">
	<div class="upload-file">
		<div class="upload-file-icon">
			<span class="icon icon-large file-icon"></span>
		</div>
		<div class="upload-file-content">
			<label>Name:</label><input name="name-${i}" value="${file.name}"/>
			<label>Description:</label><textarea  name="description-${i}"></textarea>
			<label>Overwrite:</label><span><input id="overwrite-${i}" class="checkbox" type="checkbox" name="overwrite-${i}" value="true" checked/><label for="overwrite-${i}"></label></span>
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

export function onFilesReset(event) {
	document.getElementById("upload-files").replaceChildren();
	document.getElementById("upload-button").disabled = true;
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

export default {
	updateUploadProgress,
	onFilesChange,
	onFilesReset,
	onUploadFileDelete
}
