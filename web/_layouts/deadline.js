// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

// Days remaining until the CBPR+ structured-address mandate.
//
// Computed in the browser rather than at build time, because a static page
// that says "89 days remaining" is wrong the morning after it is published,
// and a countdown that is wrong is worse than no countdown on a page whose
// whole subject is a date.
//
// The markup carries a build-time fallback, so a reader with scripting
// disabled sees a correct statement of the deadline rather than an empty box.
(function () {
  "use strict";

  var DEADLINE = Date.UTC(2026, 10, 14); // 14 November 2026, months are 0-based

  function daysRemaining() {
    var now = new Date();
    var today = Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate());
    return Math.round((DEADLINE - today) / 86400000);
  }

  function render() {
    var el = document.getElementById("days-remaining");
    var label = document.getElementById("days-label");
    if (!el) {
      return;
    }

    var days = daysRemaining();

    if (days > 0) {
      el.textContent = String(days);
      if (label) {
        label.textContent = days === 1 ? "day until the mandate" : "days until the mandate";
      }
    } else if (days === 0) {
      el.textContent = "Today";
      if (label) {
        label.textContent = "the mandate takes effect";
      }
    } else {
      el.textContent = String(-days);
      if (label) {
        label.textContent = -days === 1 ? "day since the mandate" : "days since the mandate";
      }
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", render);
  } else {
    render();
  }
})();
