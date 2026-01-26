(function () {
  "use strict";

  var urlInput = document.getElementById("urlInput");
  var addBtn = document.getElementById("addBtn");
  var urlQueueEl = document.getElementById("urlQueue");
  var syncOptions = document.getElementById("syncOptions");
  var bitrateSelect = document.getElementById("bitrateSelect");
  var syncBtn = document.getElementById("syncBtn");
  var errorMsg = document.getElementById("errorMsg");
  var progressEl = document.getElementById("progress");
  var phaseEl = document.getElementById("phase");
  var countSearched = document.getElementById("countSearched");
  var countQueued = document.getElementById("countQueued");
  var countSkipped = document.getElementById("countSkipped");
  var countNotFound = document.getElementById("countNotFound");
  var countTotal = document.getElementById("countTotal");
  var navToggle = document.getElementById("navCheck");
  var trackTable = document.getElementById("trackTable");
  var trackBody = document.getElementById("trackBody");
  var themeToggle = document.getElementById("themeToggle");
  var statsEl = document.getElementById("stats");

  var urlQueue = [];
  var pollTimer = null;
  var sessionId = null;
  var syncIndex = 0;
  var isSyncing = false;

  // Theme toggle.
  var savedTheme = localStorage.getItem("theme") || "light";
  document.documentElement.setAttribute("data-theme", savedTheme);

  themeToggle.addEventListener("click", function () {
    var current = document.documentElement.getAttribute("data-theme");
    var next = current === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    localStorage.setItem("theme", next);
  });

  // Add button click.
  addBtn.addEventListener("click", addToQueue);
  urlInput.addEventListener("keypress", function (e) {
    if (e.key === "Enter") {
      e.preventDefault();
      addToQueue();
    }
  });

  function addToQueue() {
    var url = urlInput.value.trim();
    if (!url) return;
    if (!isValidURL(url)) {
      showError("Invalid YouTube URL");
      return;
    }
    hideError();

    if (isChannelURL(url)) {
      fetchChannelPlaylists(url);
    } else {
      fetchURLInfo(url);
    }
  }

  function isValidURL(url) {
    return url.indexOf("youtube.com") !== -1 || url.indexOf("youtu.be") !== -1;
  }

  function isChannelURL(url) {
    return url.indexOf("youtube.com/@") !== -1 ||
           url.indexOf("youtube.com/channel/") !== -1 ||
           url.indexOf("youtube.com/c/") !== -1 ||
           url.indexOf("youtube.com/user/") !== -1 ||
           url.indexOf("youtube.com/browse/") !== -1;
  }

  function fetchURLInfo(url) {
    addBtn.disabled = true;
    addBtn.textContent = "loading...";

    fetch("/api/url/info?url=" + encodeURIComponent(url))
      .then(function (resp) { return resp.json(); })
      .then(function (data) {
        urlQueue.push({ url: data.url || url, title: data.title || null });
        urlInput.value = "";
        renderQueue();
      })
      .catch(function () {
        // Fallback: add with URL only
        urlQueue.push({ url: url, title: null });
        urlInput.value = "";
        renderQueue();
      })
      .finally(function () {
        addBtn.disabled = false;
        addBtn.textContent = "add";
      });
  }

  function fetchChannelPlaylists(channelURL) {
    addBtn.disabled = true;
    addBtn.textContent = "loading...";

    fetch("/api/channel/playlists?url=" + encodeURIComponent(channelURL))
      .then(function (resp) {
        if (!resp.ok) return resp.json().then(function (d) { throw new Error(d.error); });
        return resp.json();
      })
      .then(function (data) {
        if (data.playlists && data.playlists.length > 0) {
          for (var i = 0; i < data.playlists.length; i++) {
            urlQueue.push({ url: data.playlists[i].url, title: data.playlists[i].title });
          }
          renderQueue();
        } else {
          showError("No playlists found on this channel");
        }
        urlInput.value = "";
      })
      .catch(function (err) {
        showError(err.message || "Failed to fetch channel playlists");
      })
      .finally(function () {
        addBtn.disabled = false;
        addBtn.textContent = "add";
      });
  }

  function removeFromQueue(index) {
    urlQueue.splice(index, 1);
    renderQueue();
  }

  function renderQueue() {
    clearElement(urlQueueEl);
    for (var i = 0; i < urlQueue.length; i++) {
      var item = urlQueue[i];
      var li = document.createElement("li");

      var link = document.createElement("a");
      link.className = "url-text";
      link.href = item.url;
      link.target = "_blank";
      link.rel = "noopener";
      link.textContent = item.title || item.url;
      link.title = item.url;

      var btn = document.createElement("button");
      btn.className = "remove-btn";
      btn.textContent = "\u00d7";
      btn.dataset.index = i;
      btn.addEventListener("click", function () {
        removeFromQueue(parseInt(this.dataset.index, 10));
      });

      li.appendChild(link);
      li.appendChild(btn);
      urlQueueEl.appendChild(li);
    }

    // Show/hide sync options based on queue.
    if (urlQueue.length > 0) {
      syncOptions.classList.add("active");
    } else {
      syncOptions.classList.remove("active");
    }
  }

  // Sync button click.
  syncBtn.addEventListener("click", startSyncAll);

  function startSyncAll() {
    if (urlQueue.length === 0 || isSyncing) return;

    isSyncing = true;
    syncIndex = 0;
    syncBtn.disabled = true;
    addBtn.disabled = true;
    clearElement(trackBody);
    trackTable.classList.remove("active");
    progressEl.classList.add("active");
    resetCounts();
    hideError();

    syncNext();
  }

  function syncNext() {
    if (syncIndex >= urlQueue.length) {
      // All done.
      isSyncing = false;
      syncBtn.disabled = false;
      addBtn.disabled = false;
      urlQueue = [];
      renderQueue();
      return;
    }

    var item = urlQueue[syncIndex];
    var bitrate = parseInt(bitrateSelect.value, 10);
    var navEnabled = navToggle && navToggle.classList.contains("active");

    phaseEl.textContent = "syncing " + (syncIndex + 1) + "/" + urlQueue.length;

    fetch("/api/sync", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url: item.url, bitrate: bitrate, check_navidrome: navEnabled }),
    })
      .then(function (resp) {
        if (!resp.ok) return resp.json().then(function (d) { throw new Error(d.error); });
        return resp.json();
      })
      .then(function (data) {
        sessionId = data.session_id;
        startPolling();
      })
      .catch(function (err) {
        showError(err.message || "Failed to start sync");
        syncIndex++;
        syncNext();
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
          if (session.status === "error") {
            showError(session.error || "Sync failed for: " + urlQueue[syncIndex]);
          }
          syncIndex++;
          syncNext();
        }
      })
      .catch(function () {
        clearInterval(pollTimer);
        pollTimer = null;
        syncIndex++;
        syncNext();
      });
  }

  function renderSession(session) {
    var prefix = urlQueue.length > 1 ? "(" + (syncIndex + 1) + "/" + urlQueue.length + ") " : "";
    phaseEl.textContent = prefix + session.status;
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

  function resetCounts() {
    countSearched.textContent = "0";
    countQueued.textContent = "0";
    countSkipped.textContent = "0";
    countNotFound.textContent = "0";
    countTotal.textContent = "0";
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
      if (data.configured && navToggle) {
        navToggle.style.display = "block";
        if (data.skip_default) {
          navToggle.classList.add("active");
        }
        navToggle.addEventListener("click", function () {
          navToggle.classList.toggle("active");
        });
      }
    })
    .catch(function () {});

  fetchStats();
  setInterval(fetchStats, 10000);
})();
