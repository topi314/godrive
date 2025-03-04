export function tokenToClipboard(el) {
	let tokenEl = el.parentNode.closest('.table-list-entry')?.querySelector('.api-token');
	if (tokenEl) {
		navigator.clipboard.writeText(tokenEl.innerHTML)
				.then(_ => alert("Copied!"));
	}
}
