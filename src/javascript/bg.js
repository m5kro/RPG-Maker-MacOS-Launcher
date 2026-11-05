// bg.js
// Background script to guard against malware downloaders.
require("./disable-child"); // child_process load and run guard
require("./disable-net"); // Node-level http/https/net/tls/dns guard

// Only allow file:// and chrome-extension:// URLs
chrome.webRequest.onBeforeRequest.addListener(
    function (details) {
        try {
            const u = new URL(details.url);
            if (u.protocol === "file:" || u.protocol === "chrome-extension:") {
                return { cancel: false }; // allow local and extension resources
            }
            // Block everything else: http, https, ws, wss, ftp, data:, blob:, etc.
            return { cancel: true };
        } catch (_) {
            // If it's not a valid URL object, safest is to allow
            return { cancel: false };
        }
    },
    { urls: ["<all_urls>"] }, // watch every request
    ["blocking"]
);
