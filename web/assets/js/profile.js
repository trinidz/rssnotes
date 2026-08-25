document
    .getElementById("save-profile-btn")
    .addEventListener("click", async function () {
        const form = document.getElementById("profile-form");
        const formData = new FormData(form);

        // Convert to JSON if your server expects JSON
        const dataObj = Object.fromEntries(formData.entries());
        //console.log(dataObj)

        try {
            const response = await fetch("./profilesave", {
                method: "POST",
                body: JSON.stringify(dataObj),
                headers: { "Content-Type": "application/json" }, // sending JSON
            });

            if (response.ok) {
                let successTitle = "Success!";
                let successMsg = "Profile saved successfully.";
                const successData = await response.json();
                successMsg = successData.message || successMsg;
                successTitle = successData.title || successTitle;

                // SUCCESS: Alert with a "Home" button
                await showModal(`${successTitle}`, `${successMsg}`, [
                    {
                        text: "Go Home",
                        type: "primary",
                        action: () => {
                            window.location.href = "./home";
                        },
                    },
                    {
                        text: "Stay here",
                        type: "secondary",
                    },
                ]);
            } else {
                let errorMsg = "Something went wrong";
                const errorData = await response.json();
                errorMsg = errorData.message || errorData.error || errorMsg;
                await showModal("Error", `Error: ${errorMsg}`, [
                    { text: "OK", type: "secondary" },
                ]);
            }
        } catch (error) {
            // Network Error
            await showModal(
                "Network Error",
                "Could not connect to the server. Please check your connection.",
                [{ text: "OK", type: "secondary" }],
            );
        }
    });
