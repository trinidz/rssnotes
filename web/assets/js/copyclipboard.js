const gallery = document.getElementById('card-grid');
const message = document.getElementById('message');       
       
// Add click event listener to all images
gallery.addEventListener('click', async (e) => {
    if (e.target.tagName === 'IMG') {
        const textToCopy = e.target.dataset.copy;

        const card = e.target.closest('.card');
        const message = document.createElement('div');
        message.className = 'message';
        message.textContent = 'npub copied!';
        //message.textContent = `Copied!: ${textToCopy}`;
        message.style.backgroundColor = '#4CAF50';
        message.style.borderRadius='8px'

        card.appendChild(message);

        try {
            await navigator.clipboard.writeText(textToCopy);
            // Show success message
            message.style.backgroundColor = '#667eea';
            message.classList.add('show');

            // Hide message after 2 seconds
            setTimeout(() => {
                message.classList.remove('show');
            }, 2000);
        } catch (err) {
            console.error('Failed to copy:', err);
            message.textContent = 'Failed to copy!';
            message.style.backgroundColor = '#f44336';
            message.classList.add('show');

            setTimeout(() => {
                message.classList.remove('show');
            }, 2000);
        }
    }
});



