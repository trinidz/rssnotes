const progressBar = document.getElementById('progressBar');
const progressFill = document.getElementById('progressFill');
const statusText = document.getElementById('progressStatus');


// Function to set progress
function setProgress(message, width) {
    statusText.textContent = message;
    progressBar.classList.add('visible');
    progressFill.classList.add('active'); // Optional pulse
    
    if ( width >= 0 && width <= 100) {
        if (width == 100) {
            //statusText.innerHTML = "<a href='./detail'>Details</a>";
            statusText.innerHTML = "Complete!";
            //statusText.textContent = "Complete!";

            //setTimeout(() => { alert('Import Complete...Refresh page or click Details above progress bar.'); }, 500);
            progressComplete()
        }
        // Speed of progress
        progressFill.style.width = width + '%';
    } else {
        endProgress();
    } 
}

function progressComplete(successMsg) {
    let successDefault = "File imported successfully.";
    successMsg = successMsg || successDefault;

    showModal(
        "Import Complete!",
        `${successMsg}`,
        [
            {
                text: "Home",
                type: "primary",
                action: () => { window.location.href = './home'; }
            },
            {
                text: "Details",
                type: "secondary",
                action: () => { window.location.href = './detail'; }
            }
        ]
    );
}

function endProgress() {
    statusText.textContent = 'Complete...';
    progressBar.classList.remove('visible');
    progressFill.classList.remove('active');
    progressFill.style.width = '0%';
}

/* function startProgress(message, duration = 3000) {
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
} */

