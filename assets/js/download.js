function downloadFiles() {
    const dl = selectedFiles.join(',');
    window.open(`${window.location.href}?dl=${dl}`, '_blank');
}