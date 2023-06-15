import {reactive} from './petite-vue.js'
import * as api from './api.js'

export const uploadDialog = reactive({
	dir: window.location.pathname,
	files: [],
	open(files) {
		this.files.splice(0, this.files.length);
		for (const file of files) {
			this.files.push({
				file: file,
				name: file.name,
				description: '',
				progress: 0,
				error: '',
				request: null,
			});
		}
		document.querySelector("#upload-dialog").showModal();
	},
	close() {
		document.querySelector("#upload-dialog").close();
	},
	onClose() {
		for (const file of this.files) {
			if (file.request) {
				file.request.abort();
			}
		}
		this.files.splice(0, this.files.length);
	},
	toggleUploadActive(e, active) {
		e.preventDefault();
		e.stopPropagation();
		e.target.classList.toggle("active", active);
	},
	dropFiles(e) {
		this.toggleUploadActive(e, false);
		this.open(e.dataTransfer.files);
	},
	selectFiles(e) {
		e.preventDefault();
		e.stopPropagation();
		this.open(e.target.files);
	},
	removeFile(file) {
		this.files.splice(this.files.indexOf(file), 1);
	},
	upload() {
		let done = 0;
		for (const file of this.files) {
			file.request = api.uploadFile("POST",
					this.dir,
					file.file,
					undefined,
					file.name,
					file.description,
					() => {
						done++;
						if (done === this.files.length) {
							window.location.reload();
						}
					},
					(xhr) => {
						file.error = xhr.response?.message || xhr.statusText;
					},
					(e) => {
						file.progress = e.loaded / e.total;
					},
			);
		}
	},
})