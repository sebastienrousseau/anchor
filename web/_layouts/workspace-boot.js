// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Loads the engine for the workspace.
//
// Separate from workspace.js so the boot sequence is not tangled with the
// interface: this file's only job is to get the WebAssembly module running and
// to say clearly what happened if it could not, which is the failure a visitor
// is most likely to hit and least able to diagnose.
(function () {
  "use strict";

  var status = document.getElementById("ws-status");

  function fail(message) {
    if (!status) {
      return;
    }
    status.textContent = message;
    status.className = "ws-status is-error";
  }

  if (!window.WebAssembly || !WebAssembly.instantiateStreaming) {
    fail("This browser cannot stream WebAssembly, so the engine cannot run here. " +
      "The command line does the same work locally.");
    return;
  }

  var go = new window.Go();

  WebAssembly.instantiateStreaming(fetch("/askiso.wasm"), go.importObject)
    .then(function (res) {
      // go.run hands control to the Go runtime, which calls askisoReady once
      // the exported functions are in place.
      go.run(res.instance);
    })
    .catch(function (err) {
      fail("Could not load the engine: " + err);
    });
})();
