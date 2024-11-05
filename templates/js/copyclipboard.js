async function copyToClipboard(name) {
    const input = document.getElementById(name);
    input.select();
    let text = input.value;
    try {
        await navigator.clipboard.writeText(text);
    } catch (err) {
        console.error('Failed to copy: ', err);
    }
}