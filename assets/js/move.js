import {reactive} from './petite-vue.js'

export const moveDialog = reactive({
	dir: window.location.pathname,
	error: '',
	open() {
		document.querySelector('#move-dialog').showModal();
	},
	close() {
		document.querySelector('#move-dialog').close();
	},
	onClose() {
		this.error = '';
	},
	move(files) {
		const xhr = new XMLHttpRequest();
		xhr.responseType = "json";
		xhr.addEventListener("load", () => {
			if (xhr.status === 204) {
				window.location.reload();
			} else {
				this.error = xhr.response?.message || xhr.statusText;
			}
		})
		xhr.open("PUT", window.location.pathname);
		xhr.setRequestHeader("Content-Type", "application/json");
		xhr.setRequestHeader("Destination", this.dir);
		xhr.send(JSON.stringify(files));
	}
})
