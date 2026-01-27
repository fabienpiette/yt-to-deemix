(function () {
  "use strict";

  var urlInput = document.getElementById("urlInput");
  var addBtn = document.getElementById("addBtn");
  var urlQueueEl = document.getElementById("urlQueue");
  var syncOptions = document.getElementById("syncOptions");
  var bitrateSelect = document.getElementById("bitrateSelect");
  var analyzeBtn = document.getElementById("analyzeBtn");
  var downloadBtn = document.getElementById("downloadBtn");
  var errorMsg = document.getElementById("errorMsg");
  var progressEl = document.getElementById("progress");
  var phaseEl = document.getElementById("phase");
  var countSearched = document.getElementById("countSearched");
  var countSelected = document.getElementById("countSelected");
  var countQueued = document.getElementById("countQueued");
  var countSkipped = document.getElementById("countSkipped");
  var countReview = document.getElementById("countReview");
  var countNotFound = document.getElementById("countNotFound");
  var countTotal = document.getElementById("countTotal");
  var navToggle = document.getElementById("navCheck");
  var trackContainer = document.getElementById("trackContainer");
  var trackTable = document.getElementById("trackTable");
  var trackBody = document.getElementById("trackBody");
  var selectAllCheckbox = document.getElementById("selectAll");
  var themeToggle = document.getElementById("themeToggle");
  var statsEl = document.getElementById("stats");

  var urlQueue = [];
  var pollTimer = null;
  var sessionId = null;
  var syncIndex = 0;
  var isAnalyzing = false;
  var isReady = false;
  var currentTracks = [];
  var sortColumn = null;
  var sortAsc = true;

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
    // Check for duplicates
    for (var i = 0; i < urlQueue.length; i++) {
      if (urlQueue[i].url === url) {
        showError("URL already in queue");
        return;
      }
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
        renderQueue();
      })
      .catch(function () {
        // Fallback: add with URL only
        urlQueue.push({ url: url, title: null });
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

    // Show/hide sync options based on queue, ready state, or active session.
    if (urlQueue.length > 0 || isReady || currentTracks.length > 0) {
      syncOptions.classList.add("active");
    } else {
      syncOptions.classList.remove("active");
    }
  }

  // Analyze button click.
  analyzeBtn.addEventListener("click", startAnalyzeAll);

  // Download button click.
  downloadBtn.addEventListener("click", startDownload);

  // Sortable column headers.
  var sortableHeaders = trackTable.querySelectorAll("th[data-sort]");
  for (var i = 0; i < sortableHeaders.length; i++) {
    sortableHeaders[i].addEventListener("click", function () {
      var col = this.dataset.sort;
      if (sortColumn === col) {
        sortAsc = !sortAsc;
      } else {
        sortColumn = col;
        sortAsc = true;
      }
      updateSortIndicators();
      if (currentTracks.length > 0 && sessionId) {
        renderTracks(sortTracks(currentTracks), sessionId, isReady);
      }
    });
  }

  function updateSortIndicators() {
    for (var i = 0; i < sortableHeaders.length; i++) {
      var th = sortableHeaders[i];
      th.classList.remove("sort-asc", "sort-desc");
      if (th.dataset.sort === sortColumn) {
        th.classList.add(sortAsc ? "sort-asc" : "sort-desc");
      }
    }
  }

  function sortTracks(tracks) {
    if (!sortColumn) return tracks;

    var sorted = tracks.slice();
    sorted.sort(function (a, b) {
      var valA, valB;

      switch (sortColumn) {
        case "title":
          valA = a.youtube_title.toLowerCase();
          valB = b.youtube_title.toLowerCase();
          break;
        case "matched":
          valA = (a.parsed_artist + " " + a.parsed_song).toLowerCase();
          valB = (b.parsed_artist + " " + b.parsed_song).toLowerCase();
          break;
        case "result":
          valA = a.deezer_match ? (a.deezer_match.artist + " " + a.deezer_match.title).toLowerCase() : "";
          valB = b.deezer_match ? (b.deezer_match.artist + " " + b.deezer_match.title).toLowerCase() : "";
          break;
        case "confidence":
          valA = a.confidence || 0;
          valB = b.confidence || 0;
          break;
        case "status":
          valA = statusOrder(a.status);
          valB = statusOrder(b.status);
          break;
        default:
          return 0;
      }

      if (valA < valB) return sortAsc ? -1 : 1;
      if (valA > valB) return sortAsc ? 1 : -1;
      return 0;
    });

    return sorted;
  }

  function statusOrder(status) {
    var order = {
      "needs_review": 0,
      "not_found": 1,
      "error": 2,
      "found": 3,
      "queued": 4,
      "skipped": 5,
      "searching": 6,
      "pending": 7
    };
    return order[status] !== undefined ? order[status] : 99;
  }

  // Select all checkbox.
  selectAllCheckbox.addEventListener("change", function () {
    if (!isReady || !sessionId) return;
    var checked = selectAllCheckbox.checked;
    var checkboxes = trackBody.querySelectorAll("input.track-select");
    for (var i = 0; i < checkboxes.length; i++) {
      if (checkboxes[i].checked !== checked && !checkboxes[i].disabled) {
        checkboxes[i].checked = checked;
        toggleTrackSelection(sessionId, parseInt(checkboxes[i].dataset.index, 10), checked);
      }
    }
  });

  function startAnalyzeAll() {
    if (urlQueue.length === 0 || isAnalyzing) return;

    isAnalyzing = true;
    isReady = false;
    syncIndex = 0;
    currentTracks = [];
    sortColumn = null;
    sortAsc = true;
    updateSortIndicators();
    analyzeBtn.disabled = true;
    downloadBtn.classList.remove("active");
    downloadBtn.disabled = true;
    addBtn.disabled = true;
    clearElement(trackBody);
    trackContainer.classList.remove("active");
    progressEl.classList.add("active");
    resetCounts();
    hideError();

    analyzeNext();
  }

  function analyzeNext() {
    if (syncIndex >= urlQueue.length) {
      // All analyzed - if last session is ready, show download button.
      isAnalyzing = false;
      analyzeBtn.disabled = false;
      addBtn.disabled = false;
      urlQueue = [];
      renderQueue();
      return;
    }

    var item = urlQueue[syncIndex];
    var bitrate = parseInt(bitrateSelect.value, 10);
    var navEnabled = navToggle && navToggle.classList.contains("active");

    phaseEl.textContent = "analyzing " + (syncIndex + 1) + "/" + urlQueue.length;

    fetch("/api/analyze", {
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
        showError(err.message || "Failed to start analysis");
        syncIndex++;
        analyzeNext();
      });
  }

  function startDownload() {
    if (!isReady || !sessionId) return;

    isReady = false;
    analyzeBtn.disabled = true;
    downloadBtn.disabled = true;
    phaseEl.textContent = "downloading";

    fetch("/api/session/" + sessionId + "/download", { method: "POST" })
      .then(function (resp) {
        if (!resp.ok) return resp.json().then(function (d) { throw new Error(d.error); });
        startPolling();
      })
      .catch(function (err) {
        showError(err.message || "Failed to start download");
        isReady = true;
        analyzeBtn.disabled = false;
        downloadBtn.disabled = false;
      });
  }

  function startPolling() {
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = setInterval(pollSession, 800);
    pollSession();
  }

  function pollSession() {
    if (!sessionId) return;

    fetch("/api/session/" + sessionId)
      .then(function (resp) { return resp.json(); })
      .then(function (session) {
        renderSession(session);
        if (session.status === "ready") {
          clearInterval(pollTimer);
          pollTimer = null;
          isReady = true;
          isAnalyzing = false;
          analyzeBtn.disabled = false;
          downloadBtn.classList.add("active");
          downloadBtn.disabled = false;
          addBtn.disabled = false;
          syncIndex++;
          if (syncIndex < urlQueue.length) {
            // More URLs to analyze
            analyzeNext();
          }
        } else if (session.status === "done" || session.status === "error") {
          clearInterval(pollTimer);
          pollTimer = null;
          isReady = false;
          downloadBtn.classList.remove("active");
          if (session.status === "error") {
            showError(session.error || "Failed for: " + (urlQueue[syncIndex] ? urlQueue[syncIndex].url : sessionId));
          }
          syncIndex++;
          if (isAnalyzing && syncIndex < urlQueue.length) {
            analyzeNext();
          } else {
            isAnalyzing = false;
            analyzeBtn.disabled = false;
            addBtn.disabled = false;
          }
        }
      })
      .catch(function () {
        clearInterval(pollTimer);
        pollTimer = null;
        syncIndex++;
        if (isAnalyzing && syncIndex < urlQueue.length) {
          analyzeNext();
        } else {
          isAnalyzing = false;
          analyzeBtn.disabled = false;
          addBtn.disabled = false;
        }
      });
  }

  function renderSession(session) {
    var prefix = urlQueue.length > 1 ? "(" + (syncIndex + 1) + "/" + urlQueue.length + ") " : "";
    phaseEl.textContent = prefix + session.status;
    countSearched.textContent = session.progress.searched;
    countSelected.textContent = session.progress.selected;
    countQueued.textContent = session.progress.queued;
    countSkipped.textContent = session.progress.skipped;
    countReview.textContent = session.progress.needs_review;
    countNotFound.textContent = session.progress.not_found;
    countTotal.textContent = session.progress.total;

    if (session.tracks && session.tracks.length > 0) {
      trackContainer.classList.add("active");
      // Preserve original indices for API calls
      for (var i = 0; i < session.tracks.length; i++) {
        session.tracks[i]._originalIndex = i;
      }
      currentTracks = session.tracks;
      renderTracks(sortTracks(currentTracks), session.id, session.status === "ready");
    }
  }

  function renderTracks(tracks, sid, editable) {
    clearElement(trackBody);
    for (var i = 0; i < tracks.length; i++) {
      var tr = document.createElement("tr");
      var t = tracks[i];

      // Checkbox column
      var tdSelect = document.createElement("td");
      tdSelect.className = "col-select";
      var checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.className = "track-select";
      checkbox.checked = t.selected;
      checkbox.dataset.index = t._originalIndex !== undefined ? t._originalIndex : i;
      checkbox.dataset.sid = sid;
      checkbox.disabled = !editable || t.status === "skipped" || t.status === "queued";
      checkbox.addEventListener("change", function () {
        toggleTrackSelection(this.dataset.sid, parseInt(this.dataset.index, 10), this.checked);
      });
      tdSelect.appendChild(checkbox);

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
      tdResult.className = "deezer-result";
      if (t.deezer_match) {
        var resultText = t.deezer_match.artist + " - " + t.deezer_match.title;
        if (t.confidence > 0) {
          resultText += " (" + t.confidence + "%)";
        }
        var matchSpan = document.createElement("span");
        matchSpan.className = "match-text";
        matchSpan.textContent = resultText;
        tdResult.appendChild(matchSpan);

        // Add search button for needs_review tracks
        if (editable && t.status === "needs_review") {
          var searchBtn = document.createElement("button");
          searchBtn.className = "search-btn";
          searchBtn.textContent = "\u270E"; // pencil icon
          searchBtn.title = "Search for different match";
          searchBtn.dataset.index = t._originalIndex !== undefined ? t._originalIndex : i;
          searchBtn.dataset.sid = sid;
          searchBtn.addEventListener("click", function () {
            showSearchInput(this.parentElement, this.dataset.sid, parseInt(this.dataset.index, 10));
          });
          tdResult.appendChild(searchBtn);
        }
      } else if (editable && t.status === "not_found") {
        // Show search input for not found tracks
        createSearchInput(tdResult, sid, t._originalIndex !== undefined ? t._originalIndex : i);
      } else {
        tdResult.textContent = "\u2014";
      }

      var tdStatus = document.createElement("td");
      tdStatus.className = "status-icon status-" + t.status;
      tdStatus.textContent = statusIcon(t.status);
      tdStatus.title = statusTooltip(t.status);

      tr.appendChild(tdSelect);
      tr.appendChild(tdTitle);
      tr.appendChild(tdMatched);
      tr.appendChild(tdResult);
      tr.appendChild(tdStatus);
      trackBody.appendChild(tr);
    }

    // Update select all checkbox state
    updateSelectAllState();
  }

  function toggleTrackSelection(sid, index, selected) {
    fetch("/api/session/" + sid + "/track/" + index + "/select", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ selected: selected }),
    })
      .then(function (resp) {
        if (!resp.ok) return resp.json().then(function (d) { throw new Error(d.error); });
        return resp.json();
      })
      .then(function () {
        // Update the selected count
        var currentCount = parseInt(countSelected.textContent, 10);
        countSelected.textContent = selected ? currentCount + 1 : currentCount - 1;
        updateSelectAllState();
      })
      .catch(function (err) {
        showError(err.message || "Failed to update selection");
        // Revert checkbox
        var checkbox = trackBody.querySelector('input[data-index="' + index + '"]');
        if (checkbox) checkbox.checked = !selected;
      });
  }

  function createSearchInput(container, sid, index) {
    var searchInput = document.createElement("input");
    searchInput.type = "text";
    searchInput.className = "search-input";
    searchInput.placeholder = "search...";
    searchInput.dataset.index = index;
    searchInput.dataset.sid = sid;
    searchInput.addEventListener("keypress", function (e) {
      if (e.key === "Enter") {
        e.preventDefault();
        searchTrack(this.dataset.sid, parseInt(this.dataset.index, 10), this.value);
      }
    });
    container.appendChild(searchInput);
    return searchInput;
  }

  function showSearchInput(container, sid, index) {
    // Clear the container and show search input
    clearElement(container);
    var input = createSearchInput(container, sid, index);
    input.focus();
  }

  function searchTrack(sid, index, query) {
    if (!query.trim()) return;

    fetch("/api/session/" + sid + "/track/" + index + "/search", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ query: query }),
    })
      .then(function (resp) {
        if (!resp.ok) return resp.json().then(function (d) { throw new Error(d.error); });
        pollSession(); // Refresh to show new match
      })
      .catch(function (err) {
        showError(err.message || "Search failed");
      });
  }

  function updateSelectAllState() {
    var checkboxes = trackBody.querySelectorAll("input.track-select:not(:disabled)");
    var checkedCount = 0;
    for (var i = 0; i < checkboxes.length; i++) {
      if (checkboxes[i].checked) checkedCount++;
    }
    selectAllCheckbox.checked = checkboxes.length > 0 && checkedCount === checkboxes.length;
    selectAllCheckbox.indeterminate = checkedCount > 0 && checkedCount < checkboxes.length;
  }

  function statusIcon(status) {
    switch (status) {
      case "searching": return "\u22EF";
      case "found": return "\u2713";
      case "queued": return "\u2713\u2713";
      case "skipped": return "\u2205";
      case "needs_review": return "?";
      case "not_found": return "\u2717";
      case "error": return "!";
      default: return "\u2014";
    }
  }

  function statusTooltip(status) {
    switch (status) {
      case "searching": return "Searching...";
      case "found": return "Found on Deezer";
      case "queued": return "Queued for download";
      case "skipped": return "Already in library";
      case "needs_review": return "Low confidence - review match";
      case "not_found": return "Not found on Deezer";
      case "error": return "Error";
      default: return "";
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
    countSelected.textContent = "0";
    countQueued.textContent = "0";
    countSkipped.textContent = "0";
    countReview.textContent = "0";
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
