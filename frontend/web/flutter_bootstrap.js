{{flutter_js}}
{{flutter_build_config}}

// Custom loader (docs/FRONTEND.md#web-performance):
//  - CanvasKit/Skwasm served from our own origin, not Google's CDN
//    (one less DNS+TLS handshake on first load, and no IP transfer to a third party — GDPR)
//  - removes the HTML splash once Flutter has painted
//  - registers our small network-first service worker (offline app shell)
_flutter.loader.load({
  config: {
    canvasKitBaseUrl: "canvaskit/",
  },
  onEntrypointLoaded: async function (engineInitializer) {
    const appRunner = await engineInitializer.initializeEngine();
    await appRunner.runApp();
    const s = document.getElementById("splash");
    if (s) { s.style.opacity = "0"; setTimeout(() => s.remove(), 250); }
  },
});

if ("serviceWorker" in navigator && location.hostname !== "localhost") {
  window.addEventListener("load", () => navigator.serviceWorker.register("sw.js"));
}
