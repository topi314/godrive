import {reactive} from './petite-vue.js'

export const moveDialog = reactive({
	open: false,
	dir: window.location.pathname,
	error: '',
	close() {
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
