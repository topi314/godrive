import {reactive} from './petite-vue.js'
import * as api from './api.js'

export const uploadDialog = reactive({
	open: false,
	dir: window.location.pathname,
	files: [],
	toggleUploadActive(e, active) {
		e.preventDefault();
		e.stopPropagation();
		e.target.classList.toggle("active", active);
	},
	setFiles(files) {
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
	},
	dropFiles(e) {
		this.toggleUploadActive(e, false);
		this.setFiles(e.dataTransfer.files);
		this.open = true;
	},
	selectFiles(e) {
		e.preventDefault();
		e.stopPropagation();
		this.setFiles(e.target.files);
		this.open = true;
	},
	removeFile(file) {
		this.files.splice(this.files.indexOf(file), 1);
	},
	close() {
		for (const file of this.files) {
			file.request.abort();
		}
		this.files.splice(0, this.files.length);
	},
	upload() {
		let done = 0;
		for (const file of this.files) {
			api.uploadFile("POST",
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