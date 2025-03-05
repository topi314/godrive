import "./theme.js";
import "./htmx-files.js";
import {onDownloadFiles, onFileSelect, onFilesSelect} from "./files.js";
import {onFilesChange, onRemovePermissions, onUploadFileDelete, stopBubble, updateUploadProgress} from "./upload.js";
import {copyShareLink} from "./share.js";
import {tokenToClipboard} from "./copy-text.js";

window.onFilesSelect = onFilesSelect;
window.onFileSelect = onFileSelect;
window.onDownloadFiles = onDownloadFiles;

window.updateUploadProgress = updateUploadProgress;
window.onFilesChange = onFilesChange;
window.onUploadFileDelete = onUploadFileDelete;
window.onRemovePermissions = onRemovePermissions;
window.stopBubble = stopBubble;

window.copyShareLink = copyShareLink;
window.tokenToClipboard = tokenToClipboard;

htmx.defineExtension("accept-html", {
	onEvent: (name, event) => {
		if (name === "htmx:configRequest") {
			event.detail.headers["Accept"] = "text/html";
		}
	}
});
