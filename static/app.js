(function () {
  "use strict";

  const form = document.getElementById("syncForm");
  const urlInput = document.getElementById("urlInput");
  const bitrateSelect = document.getElementById("bitrateSelect");
  const syncBtn = document.getElementById("syncBtn");
  const errorMsg = document.getElementById("errorMsg");
  const progressEl = document.getElementById("progress");
  const phaseEl = document.getElementById("phase");
  const countSearched = document.getElementById("countSearched");
  const countQueued = document.getElementById("countQueued");
  const countSkipped = document.getElementById("countSkipped");
  const countNotFound = document.getElementById("countNotFound");
  const countTotal = document.getElementById("countTotal");
  const navCheck = document.getElementById("navCheck");
  const checkNavidrome = document.getElementById("checkNavidrome");
  const trackTable = document.getElementById("trackTable");
  const trackBody = document.getElementById("trackBody");
  const themeToggle = document.getElementById("themeToggle");
  const statsEl = document.getElementById("stats");

  let pollTimer = null;
  let sessionId = null;

  // Theme toggle.
  const savedTheme = localStorage.getItem("theme") || "light";
  document.documentElement.setAttribute("data-theme", savedTheme);

  themeToggle.addEventListener("click", function () {
    const current = document.documentElement.getAttribute("data-theme");
    const next = current === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    localStorage.setItem("theme", next);
  });

  // Form submit.
  form.addEventListener("submit", function (e) {
    e.preventDefault();
    startSync();
  });

  function startSync() {
    const url = urlInput.value.trim();
    if (!url) return;

    const bitrate = parseInt(bitrateSelect.value, 10);
    const navEnabled = checkNavidrome && checkNavidrome.checked;
    hideError();
    syncBtn.disabled = true;
    clearElement(trackBody);
    trackTable.classList.remove("active");
    progressEl.classList.remove("active");

    fetch("/api/sync", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url: url, bitrate: bitrate, check_navidrome: navEnabled }),
    })
      .then(function (resp) {
        if (!resp.ok) return resp.json().then(function (d) { throw new Error(d.error); });
        return resp.json();
      })
      .then(function (data) {
        sessionId = data.session_id;
        progressEl.classList.add("active");
        startPolling();
      })
      .catch(function (err) {
        showError(err.message || "Failed to start sync");
        syncBtn.disabled = false;
      });
  }

  function startPolling() {
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = setInterval(pollSession, 800);
    pollSession();
  }

  function pollSession() {
    if (!sessionId) return;

    fetch("/api/sync/" + sessionId)
      .then(function (resp) { return resp.json(); })
      .then(function (session) {
        renderSession(session);
        if (session.status === "done" || session.status === "error") {
          clearInterval(pollTimer);
          pollTimer = null;
          syncBtn.disabled = false;
          if (session.status === "error") {
            showError(session.error || "Sync failed");
          }
        }
      })
      .catch(function () {
        clearInterval(pollTimer);
        pollTimer = null;
        syncBtn.disabled = false;
      });
  }

  function renderSession(session) {
    phaseEl.textContent = session.status;
    countSearched.textContent = session.progress.searched;
    countQueued.textContent = session.progress.queued;
    countSkipped.textContent = session.progress.skipped;
    countNotFound.textContent = session.progress.not_found;
    countTotal.textContent = session.progress.total;

    if (session.tracks && session.tracks.length > 0) {
      trackTable.classList.add("active");
      renderTracks(session.tracks);
    }
  }

  function renderTracks(tracks) {
    clearElement(trackBody);
    for (var i = 0; i < tracks.length; i++) {
      var tr = document.createElement("tr");
      var t = tracks[i];

      var tdTitle = document.createElement("td");
      tdTitle.textContent = truncate(t.youtube_title, 40);
      tdTitle.title = t.youtube_title;

      var tdMatched = document.createElement("td");
      if (t.parsed_artist) {
        tdMatched.textContent = t.parsed_artist + " - " + t.parsed_song;
      } else {
        tdMatched.textContent = t.parsed_song;
      }

      var tdResult = document.createElement("td");
      if (t.deezer_match) {
        tdResult.textContent = t.deezer_match.artist + " - " + t.deezer_match.title;
      } else {
        tdResult.textContent = "\u2014";
      }

      var tdStatus = document.createElement("td");
      tdStatus.className = "status-icon";
      tdStatus.textContent = statusIcon(t.status);

      tr.appendChild(tdTitle);
      tr.appendChild(tdMatched);
      tr.appendChild(tdResult);
      tr.appendChild(tdStatus);
      trackBody.appendChild(tr);
    }
  }

  function statusIcon(status) {
    switch (status) {
      case "searching": return "\u22EF";
      case "found":
      case "queued": return "\u2713";
      case "skipped": return "\u2205";
      case "not_found": return "\u2717";
      case "error": return "!";
      default: return "\u2014";
    }
  }

  function truncate(str, len) {
    if (str.length <= len) return str;
    return str.substring(0, len - 1) + "\u2026";
  }

  function clearElement(el) {
    while (el.firstChild) {
      el.removeChild(el.firstChild);
    }
  }

  function showError(msg) {
    errorMsg.textContent = msg;
    errorMsg.classList.add("active");
  }

  function hideError() {
    errorMsg.textContent = "";
    errorMsg.classList.remove("active");
  }

  // Stats polling.
  function fetchStats() {
    fetch("/api/stats")
      .then(function (resp) { return resp.json(); })
      .then(function (data) {
        var mem = data.memory_mb.toFixed(1);
        var uptime = formatUptime(data.uptime_sec);
        statsEl.textContent = mem + " MB / " + data.goroutines + " goroutines / up " + uptime;
      })
      .catch(function () {});
  }

  function formatUptime(sec) {
    var h = Math.floor(sec / 3600);
    var m = Math.floor((sec % 3600) / 60);
    if (h > 0) return h + "h " + m + "m";
    if (m > 0) return m + "m";
    return Math.floor(sec) + "s";
  }

  // Check Navidrome availability.
  fetch("/api/navidrome/status")
    .then(function (resp) { return resp.json(); })
    .then(function (data) {
      if (data.configured) {
        navCheck.style.display = "";
      }
    })
    .catch(function () {});

  fetchStats();
  setInterval(fetchStats, 10000);
})();
