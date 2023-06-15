import {reactive} from './petite-vue.js'
import * as api from './api.js'

export const editDialog = reactive({
	name: '',
	dir: '',
	newName: '',
	description: '',
	file: null,
	request: null,
	progress: 0,
	error: '',
	open(name, dir, newName, description) {
		this.name = name;
		this.dir = dir;
		this.newName = newName;
		this.description = description;
		document.querySelector("#edit-dialog").showModal();
	},
	close() {
		document.querySelector("#edit-dialog").close();
	},
	selectFile(e) {
		this.file = e.target.files[0];
		if (this.file.name !== this.newName) {
			this.newName = this.file.name;
		}
	},
	onClose() {
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
		this.request = api.uploadFile("PATCH",
				path + this.name,
				this.file,
				this.dir,
				this.newName,
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