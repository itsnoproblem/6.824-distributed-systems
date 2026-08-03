// Exercise editor bootstrap. Depends on window.CM (vendored CodeMirror
// bundle) and htmx. Idempotent: re-scans after every htmx swap.
(function () {
  "use strict";

  function debounce(fn, ms) {
    var t;
    return function () { clearTimeout(t); t = setTimeout(fn, ms); };
  }

  function init(root) {
    root.dataset.initialized = "1";
    var cfg = JSON.parse(root.dataset.config);
    var tabbar = root.querySelector("#exercise-tabs");
    var host = root.querySelector("#exercise-editor");
    var saveError = root.querySelector("#exercise-save-error");
    var CM = window.CM;
    var states = {};
    var current = null;
    var view = null;

    function mkState(file) {
      return CM.EditorState.create({
        doc: file.content,
        extensions: [
          CM.basicSetup,
          CM.go(),
          CM.lintGutter(),
          CM.EditorView.editable.of(!file.readonly),
          CM.EditorState.readOnly.of(file.readonly),
          CM.EditorView.updateListener.of(function (u) {
            if (u.docChanged) { onChange(); }
          }),
        ],
      });
    }

    function editableFiles() {
      var out = {};
      cfg.files.forEach(function (f) {
        if (f.readonly) { return; }
        out[f.name] = f.name === current
          ? view.state.doc.toString()
          : states[f.name].doc.toString();
      });
      return out;
    }

    function save() {
      return fetch(cfg.saveUrl, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ files: editableFiles() }),
      }).then(function (r) {
        if (!r.ok) { throw new Error("save failed: " + r.status); }
        if (saveError) { saveError.hidden = true; }
      }).catch(function () {
        // transient; next change retries — surface it so keystrokes never
        // look silently dropped.
        if (saveError) { saveError.hidden = false; }
      });
    }

    function check() {
      fetch(cfg.checkUrl, { method: "POST" })
        .then(function (r) { return r.json(); })
        .then(function (diags) {
          var forCurrent = diags
            .filter(function (d) { return d.file === current; })
            .map(function (d) {
              var line = view.state.doc.line(Math.min(d.line, view.state.doc.lines));
              var from = Math.min(line.from + Math.max(d.col - 1, 0), line.to);
              return { from: from, to: line.to, severity: "error", message: d.message };
            });
          view.dispatch(CM.setDiagnostics(view.state, forCurrent));
        })
        .catch(function (err) {
          // Transient; not surfaced in the UI, but keep it visible in devtools.
          console.warn("exercise check failed:", err);
        });
    }

    var debouncedSave = debounce(save, 800);
    var debouncedCheck = debounce(function () { save().then(check); }, 1500);
    function onChange() { debouncedSave(); debouncedCheck(); }

    function show(name) {
      if (view && current) { states[current] = view.state; }
      current = name;
      if (!view) {
        view = new CM.EditorView({ state: states[name], parent: host });
      } else {
        view.setState(states[name]);
      }
      tabbar.querySelectorAll("button").forEach(function (b) {
        b.classList.toggle("active", b.dataset.file === name);
      });
    }

    cfg.files.forEach(function (f) {
      states[f.name] = mkState(f);
      var b = document.createElement("button");
      b.type = "button";
      b.className = "tab" + (f.readonly ? " readonly" : "");
      b.dataset.file = f.name;
      b.textContent = f.readonly ? f.name + " (read-only)" : f.name;
      b.addEventListener("click", function () { show(f.name); });
      tabbar.appendChild(b);
    });
    show(cfg.files[0].name);

    var runBtn = root.querySelector("#exercise-run");
    runBtn.addEventListener("click", function () {
      runBtn.disabled = true;
      save().then(function () {
        return window.htmx.ajax("POST", cfg.runUrl,
          { target: "#exercise-status", swap: "innerHTML" });
      }).finally(function () { runBtn.disabled = false; });
    });

    root.addEventListener("keydown", function (e) {
      if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
        e.preventDefault();
        runBtn.click();
      }
    });
  }

  function scan() {
    document.querySelectorAll("#exercise-root:not([data-initialized])")
      .forEach(function (root) { if (window.CM) { init(root); } });
  }
  scan();
  document.body.addEventListener("htmx:afterSettle", scan);
})();
