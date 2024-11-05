const rsslayPubKeyStorageKey = "rsslay.publicKey";
const rsslayRelaysStorageKey = "rsslay.relays";

const loginButton = document.getElementById('login');
const logoutButton = document.getElementById('logout');
const loginButtonText = document.getElementById('login-text');

let pubKey;
let relays;
let relaysUrls;
let subs;
let followListEvent;
let pool;

function tryAddToFollowList(pubKeyToFollow) {
    if (followListEvent) {
        const newFollowTag = ["p", pubKeyToFollow];
        const tagsSet = new Set(followListEvent.tags);
        let found = false;
        followListEvent.tags.forEach((value) => {
            if (value[1] === pubKeyToFollow){
                found = true;
            }
        });
        if (found){
            return followListEvent.tags;
        }
        tagsSet.add(newFollowTag);
        return [...tagsSet];
    } else {
        swal({
            title: "Connecting...",
            text: "Waiting for relays to retrieve your contact list. Please wait a few seconds and try again...",
            icon: "error",
            button: "Ok",
        });
    }
}

function tryRemoveFromFollowList(publicKeyToUnfollow) {
    if (followListEvent) {
        const tagsSet = new Set();
        followListEvent.tags.forEach((value) => {
            if (value[1] !== publicKeyToUnfollow){
                tagsSet.add(value);
            }
        });
        return [...tagsSet];
    } else {
        swal({
            title: "Connecting...",
            text: "Waiting for relays to retrieve your contact list. Please wait a few seconds and try again...",
            icon: "error",
            button: "Ok",
        });
    }
}

async function tryUnfollow(pubKey) {
    if (pubKey) {
        const loggedIn = checkLogin();
        if (!loggedIn){
            await performLogin();
        }
        let event = {
            kind: 3,
            created_at: Math.floor(Date.now() / 1000),
            tags: tryRemoveFromFollowList(pubKey),
            content: JSON.stringify(relays),
        }
        const signedEvent = await window.nostr.signEvent(event);

        let ok = window.NostrTools.validateEvent(signedEvent);
        let veryOk = window.NostrTools.verifySignature(signedEvent);
        if (ok && veryOk){
            let alerted = false;
            let pubs = pool.publish(relaysUrls, signedEvent);
            pubs.forEach(pub => {
                pub.on('ok', () => {
                    if (!alerted) {
                        alerted = true;
                        swal({
                            title: "Not following",
                            text: "Your contact list has been updated, you're now no longer following the profile.",
                            icon: "success",
                            button: "Ok",
                        });
                        const followButton = document.getElementById(pubKey);
                        followButton.classList.remove("is-danger");
                        followButton.classList.add("is-link");
                        followButton.setAttribute('onclick', `tryFollow("${pubKey}")`);
                        followButton.textContent = "Follow profile";
                    }
                })
                pub.on('seen', () => {
                    console.log(`we saw the event!`);
                })
                pub.on('failed', reason => {
                    console.log(`failed to publish: ${reason}`);
                })
            });
        }
    }
}

async function tryFollow(pubKey) {
    if (pubKey) {
        const loggedIn = checkLogin();
        if (!loggedIn){
            await performLogin();
        }
        let event = {
            kind: 3,
            created_at: Math.floor(Date.now() / 1000),
            tags: tryAddToFollowList(pubKey),
            content: JSON.stringify(relays),
        }
        const signedEvent = await window.nostr.signEvent(event);

        let ok = window.NostrTools.validateEvent(signedEvent);
        let veryOk = window.NostrTools.verifySignature(signedEvent);
        if (ok && veryOk){
            let alerted = false;
            let pubs = pool.publish(relaysUrls, signedEvent);
            pubs.forEach(pub => {
                pub.on('ok', () => {
                    if (!alerted){
                        alerted = true;
                        swal({
                            title: "Followed!",
                            text: "Your contact list has been updated, you're now following the profile.",
                            icon: "success",
                            button: "Ok",
                        });
                        const followButton = document.getElementById(pubKey);
                        followButton.classList.remove("is-link");
                        followButton.classList.add("is-danger");
                        followButton.setAttribute('onclick', `tryUnfollow("${pubKey}")`);
                        followButton.textContent = "Unfollow";
                    }
                })
                pub.on('seen', () => {
                    console.log(`we saw the event!`);
                })
                pub.on('failed', reason => {
                    console.log(`failed to publish: ${reason}`);
                })
            });
        }
    }
}

function parseFollowList(followListEvent) {
    const profilesPubKeys = new Set();
    followListEvent.tags.forEach((tag) => {
        profilesPubKeys.add(tag[1]);
    });
    profilesPubKeys.forEach((pubKey) => {
        const followButton = document.getElementById(pubKey);
        if (followButton) {
            followButton.classList.remove("is-link");
            followButton.classList.add("is-danger");
            followButton.setAttribute('onclick', `tryUnfollow("${pubKey}")`);
            followButton.textContent = "Unfollow";
        }
    });
}

function connectToRelays(){
    pool = new window.NostrTools.SimplePool()

    subs = pool.sub([...relaysUrls], [{
        authors: [pubKey],
        kind: 3,
    }]);

    subs.on('event', event => {
        if (event.kind === 3){
            followListEvent = event;
            relays = JSON.parse(event.content);
            relaysUrls = Object.keys(relays);
            sessionStorage.setItem(rsslayRelaysStorageKey, JSON.stringify(relays));
            parseFollowList(followListEvent);
        }
    });
}

function checkLogin(){
    if (typeof window.nostr !== 'undefined') {
        pubKey = sessionStorage.getItem(rsslayPubKeyStorageKey);
        relays = sessionStorage.getItem(rsslayRelaysStorageKey);
        if (pubKey && relays){
            relays = JSON.parse(relays);
            relaysUrls = Object.keys(relays);
            afterLogin();
            return true;
        }
    }
    return false;
}

async function performLogin() {
    if (typeof window.nostr !== 'undefined') {
        pubKey = sessionStorage.getItem(rsslayPubKeyStorageKey);
        relays = sessionStorage.getItem(rsslayRelaysStorageKey);
        if (!pubKey || !relays){
            try {
                pubKey = await window.nostr.getPublicKey();
                relays = await window.nostr.getRelays();
                sessionStorage.setItem(rsslayPubKeyStorageKey, pubKey);
                sessionStorage.setItem(rsslayRelaysStorageKey, JSON.stringify(relays));
            } catch (e) {
                swal({
                    title: "Oops...",
                    text: "There was a problem to obtain public info from your profile... Try again later and make sure to grant correct permissions in your extension",
                    icon: "error",
                    button: "Ok",
                });
                return;
            }
        } else {
            relays = JSON.parse(relays);
        }
        relaysUrls = Object.keys(relays);
        afterLogin();
    } else {
        swal({
            title: "Oops...",
            text: "There was a problem to obtain your public key... Try again later and make sure to grant correct permissions in your extension",
            icon: "error",
            button: "Ok",
        });
    }
}

async function performLogout() {
    loginButton.disabled = false;
    loginButton.addEventListener('click', performLogin);
    loginButtonText.textContent = "Login";
    logoutButton.disabled = true;
    logoutButton.removeEventListener('click', performLogout);
    sessionStorage.clear();
    subs.unsub();
    pool.close([...relaysUrls]);
}

function afterLogin() {
    loginButton.disabled = true;
    loginButton.removeEventListener('click', performLogin);
    loginButtonText.textContent = "Logged in!";
    logoutButton.disabled = false;
    logoutButton.addEventListener('click', performLogout);
    connectToRelays();
}