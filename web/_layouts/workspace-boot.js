// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Loads the engine for the workspace.
//
// Separate from workspace.js so the boot sequence is not tangled with the
// interface: this file's only job is to get the WebAssembly module running and
// to say clearly what happened if it could not, which is the failure a visitor
// is most likely to hit and least able to diagnose.
//
// It loads on the first sign of intent rather than on page load. The module is
// 5.7 MB and instantiating it blocks the main thread for a couple of hundred
// milliseconds; somebody who opened the page to read what it does should not
// pay for either. In practice the first scroll, tap or keypress starts it, so
// it is usually running before anyone reaches the input box.
(function () {
  "use strict";

  var status = document.getElementById("ws-status");
  var starting = false;

  function fail(message) {
    if (!status) {
      return;
    }
    status.textContent = message;
    status.className = "ws-status is-error";
  }

  function start() {
    if (starting) {
      return;
    }
    starting = true;

    if (!window.WebAssembly || !WebAssembly.instantiateStreaming) {
      fail("This browser cannot stream WebAssembly, so the engine cannot run here. " +
        "The command line does the same work locally.");
      return;
    }

    if (status) {
      status.textContent = "Loading the engine…";
      status.className = "ws-status";
    }

    var go = new window.Go();

    WebAssembly.instantiateStreaming(fetch("/askiso.wasm"), go.importObject)
      .then(function (res) {
        // go.run hands control to the Go runtime, which calls askisoReady once
        // the exported functions are in place.
        go.run(res.instance);
      })
      .catch(function (err) {
        starting = false;
        fail("Could not load the engine: " + err);
      });
  }

  // Exposed so the interface can start the engine itself when somebody acts
  // before the first of these events has fired.
  window.askisoStartEngine = start;

  ["pointerdown", "keydown", "focusin"].forEach(function (evt) {
    document.addEventListener(evt, start, { once: true, passive: true });
  });
})();
