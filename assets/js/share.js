import {reactive} from './petite-vue.js'

export const shareDialog = reactive({
	permissions: [],
	path: "",
	error: "",
	open(path) {
		this.path = path;
		document.querySelector("#share-dialog").showModal();
	},
	close() {
		document.querySelector("#share-dialog").close();
	},
	onClose() {
		this.permissions.splice(0, this.permissions.length);
		this.path = "";
		this.error = "";
	},
	copy() {
		const xhr = new XMLHttpRequest();
		xhr.responseType = "json";
		xhr.addEventListener("load", async () => {
			if (xhr.status === 200) {
				const link = `${this.window.location.href}?${xhr.response.token}`;
				await navigator.clipboard.writeText(link)
				this.error = "Link copied to clipboard";
			} else {
				this.error = xhr.response?.message || xhr.statusText;
			}
		})
		xhr.open("POST", "/api/share");
		xhr.setRequestHeader("Content-Type", "application/json");
		xhr.send(JSON.stringify({
			path: this.path,
			permissions: this.permissions,
		}));
	},
})