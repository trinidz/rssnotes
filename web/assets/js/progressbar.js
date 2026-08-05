const progressBar = document.getElementById('progressBar');
const progressFill = document.getElementById('progressFill');
const statusText = document.getElementById('progressStatus');

// Function to start progress
function startProgress(message, duration = 3000) {
    statusText.textContent = message;
    progressBar.classList.add('visible');
    progressFill.classList.add('active'); // Optional pulse
    
    let width = 0;
    const interval = setInterval(() => {
        if (width >= 100) {
            clearInterval(interval);
            endProgress();
        } else {
            width += 2; // Speed of progress
            progressFill.style.width = width + '%';
        }
    }, duration / 50);
}

// Function to set progress
function setProgress(message, width) {
    statusText.textContent = message;
    progressBar.classList.add('visible');
    progressFill.classList.add('active'); // Optional pulse
    
    if ( width >= 0 && width <= 100) {
        if (width == 100) {
            statusText.innerHTML = "<a href='./detail'>Details</a>";
            //statusText.textContent = "Complete!";
            setTimeout(() => { alert('Import Complete...Refresh page or click Details above progress bar.'); }, 500);
        }
        // Speed of progress
        progressFill.style.width = width + '%';
    } else {
        endProgress();
    } 
}

// Function to end progress
function endProgress() {
    statusText.textContent = 'Complete...';
    progressBar.classList.remove('visible');
    progressFill.classList.remove('active');
    progressFill.style.width = '0%';
}

// Example Usage: Call this when an action starts
// startProgress('Uploading Data...', 3000);

// <script>
//    document.getElementById('progressBar').classList.add('visible');
//    document.querySelector('.progress-fill').style.width = '75%';
// </script>