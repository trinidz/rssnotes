/**
 * Shows a custom modal.
 * @param {string} title - Modal title
 * @param {string} message - Modal body text
 * @param {Array} buttons - Array of objects: { text: 'Btn', type: 'primary'|'secondary', action: function }
 * @returns {Promise} - Resolves with the clicked button's action or null if closed
 */
function showModal(title, message, buttons = []) {
  return new Promise((resolve) => {
    const modal = document.getElementById('custom-modal');
    const titleEl = document.getElementById('modal-title');
    const msgEl = document.getElementById('modal-message');
    const footerEl = document.getElementById('modal-footer');
    const closeBtn = document.getElementById('modal-close');

    // Set content
    titleEl.textContent = title;
    msgEl.textContent = message;
    footerEl.innerHTML = ''; // Clear previous buttons

    // Create buttons
    buttons.forEach(btnConfig => {
      const btn = document.createElement('button');
      btn.textContent = btnConfig.text;
      btn.className = `modal-btn ${btnConfig.type || 'secondary'}`;
      
      btn.onclick = () => {
        hideModal();
        if (btnConfig.action) btnConfig.action();
        resolve(btnConfig.text); // Resolve promise with button text
      };
      footerEl.appendChild(btn);
    });

    // Close on X click
    closeBtn.onclick = () => {
      hideModal();
      resolve(null);
    };

    // Close on backdrop click
    modal.onclick = (e) => {
      if (e.target === modal) {
        hideModal();
        resolve(null);
      }
    };

    // Show modal
    modal.style.display = 'flex';
    // Small timeout to allow display:flex to apply before adding opacity class
    setTimeout(() => modal.classList.add('show'), 10);
  });
}

function hideModal() {
  const modal = document.getElementById('custom-modal');
  modal.classList.remove('show');
  setTimeout(() => {
    modal.style.display = 'none';
  }, 300); // Match CSS transition time
}