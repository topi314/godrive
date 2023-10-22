export async function copyShareLink() {
	const shareLink = document.getElementById("share-link");

	shareLink.select();
	shareLink.setSelectionRange(0, 99999);

	await navigator.clipboard.writeText(shareLink.value);
}

export default {
	copyShareLink
}