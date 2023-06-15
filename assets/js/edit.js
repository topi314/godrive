import {reactive} from './petite-vue.js'
import * as api from './api.js'

export const editDialog = reactive({
	open: false,
	editFile: '',
	dir: '',
	name: '',
	description: '',
	file: null,
	request: null,
	progress: 0,
	error: '',
	selectFile(e) {
		this.file = e.target.files[0];
		if (this.editFile === this.name) {
			this.name = this.file.name;
		}
	},
	close() {
		if (this.request) {
			this.request.abort();
		}
		this.file = null;
		this.name = '';
		this.description = '';
		this.progress = 0;
		this.error = '';
		this.request = null;
	},
	edit() {
		let path = window.location.pathname;
		if (!path.endsWith("/")) {
			path += "/";
		}
		api.uploadFile("PATCH",
				path + this.editFile,
				this.file,
				this.dir,
				this.name,
				this.description,
				() => {
					window.location.reload();
				},
				(xhr) => {
					this.error = xhr.response?.message || xhr.statusText;
				},
				(e) => {
					this.progress = e.loaded / e.total;
				},
		);
	},
})