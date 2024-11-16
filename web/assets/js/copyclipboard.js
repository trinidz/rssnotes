async function copyToClipboard(name) {
    const text = document.getElementById(name).value;
    try {
        await navigator.clipboard.writeText(text);
    } catch (err) {
        console.error('Failed to copy: ', err);
    }
}