// Live run pane: tails the SSE stream into the output pre, offers cancel,
// and swaps in the final section when the run completes. Initialization is
// idempotent — the script tag rides along with every htmx swap of the pane.
(function () {
  "use strict";

  function init(pane) {
    if (pane.dataset.init) { return; }
    pane.dataset.init = "1";

    var out = pane.querySelector(".run-live-output");
    var cancelBtn = pane.querySelector(".run-live-cancel");
    var refreshed = false;
    var es = new EventSource(pane.dataset.streamUrl);

    // Loads the authoritative section state exactly once; the server decides
    // what renders next (result, LLM-phase polling, or failure).
    function refresh() {
      if (refreshed) { return; }
      refreshed = true;
      es.close();
      window.htmx.ajax("GET", pane.dataset.sectionUrl,
        { target: pane.dataset.target, swap: "innerHTML" });
    }

    es.addEventListener("chunk", function (ev) {
      out.textContent += JSON.parse(ev.data);
      out.scrollTop = out.scrollHeight;
    });
    es.addEventListener("dropped", function () {
      out.textContent += "…(earlier output dropped)…\n";
    });
    es.addEventListener("done", function () {
      // Small delay lets the pipeline persist its final status before the
      // section re-render reads it.
      setTimeout(refresh, 500);
    });
    es.onerror = function () {
      // Broken stream (server restart, proxy hiccup): fall back to a single
      // delayed section reload — the server-rendered result includes the
      // polling fallback if the run is still going.
      setTimeout(refresh, 3000);
    };

    cancelBtn.addEventListener("click", function () {
      cancelBtn.disabled = true;
      fetch(pane.dataset.cancelUrl, { method: "POST" }).then(function (res) {
        if (!res.ok) { cancelBtn.disabled = false; }
      }).catch(function () {
        cancelBtn.disabled = false;
      });
      // No UI update here: cancellation surfaces through the stream's done
      // event and the section re-render.
    });
  }

  document.querySelectorAll(".run-live").forEach(init);
})();
