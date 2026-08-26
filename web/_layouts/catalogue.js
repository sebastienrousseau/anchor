// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Reads the visitor's own ISO 20022 catalogue from their own disk.
//
// This is the one honest way to offer full schema validation in a browser on a
// site that redistributes no specification content. AskISO does not host the
// schemas and will not; the File System Access API lets the page read the copy
// the user already downloaded from the Registration Authority. Nothing is
// uploaded, nothing is cached on a server, and the handle is dropped when the
// tab closes.
//
// The API is Chromium-only today. Everywhere else the page keeps working
// exactly as before — paste the schema by hand — and says so rather than
// offering a button that does nothing.
(function () {
  "use strict";

  // Directory layout the Registration Authority's archives unpack into, and
  // the one `askiso catalog add` produces:
  //
  //   <root>/<Category>/Version N.0/Schemas/<message-id>.xsd
  var VERSION_DIR = /^Version\b/i;
  var SCHEMA_DIR = "Schemas";

  // Namespace of an ISO 20022 document names the message it is:
  //   urn:iso:std:iso:20022:tech:xsd:pacs.008.001.10
  var NS = /urn:iso:std:iso:20022:tech:xsd:([A-Za-z]+\.\d+\.\d+\.\d+)/;

  var index = null; // message id -> FileSystemFileHandle
  var rootName = "";

  function supported() {
    return typeof window.showDirectoryPicker === "function";
  }

  function $(sel) {
    return document.querySelector(sel);
  }

  function say(msg, kind) {
    var el = $("#cat-status");
    if (!el) {
      return;
    }
    el.textContent = msg;
    el.className = "cat-status" + (kind ? " is-" + kind : "");
  }

  // Walk <root>/<Category>/<Version N>/Schemas and record every .xsd. The walk
  // is deliberately shallow: three levels is the published layout, and
  // recursing an arbitrary directory a user picked would be both slow and
  // presumptuous.
  async function buildIndex(root) {
    var found = new Map();

    for await (var category of root.values()) {
      if (category.kind !== "directory" || category.name.startsWith(".")) {
        continue;
      }
      for await (var version of category.values()) {
        if (version.kind !== "directory" || !VERSION_DIR.test(version.name)) {
          continue;
        }

        var schemas;
        try {
          schemas = await version.getDirectoryHandle(SCHEMA_DIR);
        } catch (e) {
          continue; // a version without a Schemas directory is not an error
        }

        for await (var entry of schemas.values()) {
          if (entry.kind === "file" && entry.name.toLowerCase().endsWith(".xsd")) {
            found.set(entry.name.replace(/\.xsd$/i, ""), entry);
          }
        }
      }
    }

    return found;
  }

  async function choose() {
    var root;
    try {
      root = await window.showDirectoryPicker({ id: "askiso-catalogue", mode: "read" });
    } catch (e) {
      // AbortError is the user closing the picker, which is not a failure.
      if (e && e.name !== "AbortError") {
        say("Could not open that directory: " + e.message, "error");
      }
      return;
    }

    say("Reading " + root.name + "…");

    try {
      index = await buildIndex(root);
    } catch (e) {
      say("Could not read that directory: " + e.message, "error");
      return;
    }

    rootName = root.name;

    if (index.size === 0) {
      say(
        'No schemas found in "' + rootName + '". Expected <Category>/Version N/Schemas/*.xsd — ' +
        "pick the folder that contains the message set directories.",
        "error"
      );
      index = null;
      return;
    }

    say(index.size + " schema(s) read from " + rootName + " — stays on your machine", "ready");
    document.body.classList.add("has-catalogue");
  }

  function forget() {
    index = null;
    rootName = "";
    document.body.classList.remove("has-catalogue");
    say(supported() ? "No catalogue selected." : unsupportedMessage());
  }

  // messageIDFrom returns the message a document declares itself to be.
  function messageIDFrom(xml) {
    var m = NS.exec(xml || "");
    return m ? m[1] : "";
  }

  // schemaFor resolves the schema text for a document, or "" when the
  // catalogue does not hold it. Exposed on window so the playground's validate
  // handler can use it without this module knowing about the panel.
  async function schemaFor(xml) {
    if (!index) {
      return "";
    }
    var id = messageIDFrom(xml);
    if (!id) {
      return "";
    }
    var handle = index.get(id);
    if (!handle) {
      return "";
    }
    var file = await handle.getFile();
    return file.text();
  }

  function unsupportedMessage() {
    return (
      "Your browser cannot read a local folder (the File System Access API is " +
      "Chromium-only today). Paste the schema below, or use the command line, " +
      "which reads your catalogue directly."
    );
  }

  function init() {
    var button = $("#cat-pick");
    if (!button) {
      return;
    }

    if (!supported()) {
      button.disabled = true;
      say(unsupportedMessage());
      return;
    }

    button.addEventListener("click", choose);

    var clear = $("#cat-forget");
    if (clear) {
      clear.addEventListener("click", forget);
    }

    say("No catalogue selected.");
  }

  window.askisoCatalogue = {
    schemaFor: schemaFor,
    messageIDFrom: messageIDFrom,
    count: function () {
      return index ? index.size : 0;
    },
    name: function () {
      return rootName;
    },
    supported: supported,
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
