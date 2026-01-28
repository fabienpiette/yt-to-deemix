(function () {
  "use strict";

  var urlInput = document.getElementById("urlInput");
  var addBtn = document.getElementById("addBtn");
  var urlQueueEl = document.getElementById("urlQueue");
  var syncOptions = document.getElementById("syncOptions");
  var bitrateSelect = document.getElementById("bitrateSelect");
  var analyzeBtn = document.getElementById("analyzeBtn");
  var downloadBtn = document.getElementById("downloadBtn");
  var cancelBtn = document.getElementById("cancelBtn");
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
  var sessionIds = [];
  var currentSessionId = null;
  var syncIndex = 0;
  var isAnalyzing = false;
  var isReady = false;
  var isPaused = false;
  var currentTracks = [];
  var totalProgress = { searched: 0, selected: 0, queued: 0, skipped: 0, needs_review: 0, not_found: 0, total: 0 };
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
    addBtn.classList.add("loading");
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
        addBtn.classList.remove("loading");
        addBtn.textContent = "add";
      });
  }

  function fetchChannelPlaylists(channelURL) {
    addBtn.disabled = true;
    addBtn.classList.add("loading");
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
        addBtn.classList.remove("loading");
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

  // Analyze button click - toggles between analyze/pause/resume.
  analyzeBtn.addEventListener("click", handleAnalyzeClick);

  // Download button click.
  downloadBtn.addEventListener("click", startDownload);

  // Cancel button click.
  cancelBtn.addEventListener("click", cancelAllSessions);

  function handleAnalyzeClick() {
    if (isPaused) {
      // Resume
      resumeAllSessions();
    } else if (isAnalyzing) {
      // Pause
      pauseAllSessions();
    } else {
      // Start new analysis
      startAnalyzeAll();
    }
  }

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
      if (currentTracks.length > 0) {
        renderTracks(sortTracks(currentTracks), null, isReady);
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
    if (!isReady) return;
    var checked = selectAllCheckbox.checked;
    var checkboxes = trackBody.querySelectorAll("input.track-select");
    for (var i = 0; i < checkboxes.length; i++) {
      if (checkboxes[i].checked !== checked && !checkboxes[i].disabled) {
        checkboxes[i].checked = checked;
        toggleTrackSelection(checkboxes[i].dataset.sid, parseInt(checkboxes[i].dataset.index, 10), checked);
      }
    }
  });

  function startAnalyzeAll() {
    if (urlQueue.length === 0 || isAnalyzing) return;

    isAnalyzing = true;
    isReady = false;
    isPaused = false;
    syncIndex = 0;
    sessionIds = [];
    currentSessionId = null;
    currentTracks = [];
    totalProgress = { searched: 0, selected: 0, queued: 0, skipped: 0, needs_review: 0, not_found: 0, total: 0 };
    sortColumn = null;
    sortAsc = true;
    updateSortIndicators();
    downloadBtn.classList.remove("active");
    downloadBtn.disabled = true;
    addBtn.disabled = true;
    clearElement(trackBody);
    trackContainer.classList.remove("active");
    progressEl.classList.add("active");
    resetCounts();
    hideError();
    updateControlButtons();

    analyzeNext();
  }

  function analyzeNext() {
    if (syncIndex >= urlQueue.length) {
      // All analyzed - if last session is ready, show download button.
      isAnalyzing = false;
      addBtn.disabled = false;
      updateControlButtons();
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
        currentSessionId = data.session_id;
        sessionIds.push(data.session_id);
        startPolling();
      })
      .catch(function (err) {
        showError(err.message || "Failed to start analysis");
        syncIndex++;
        analyzeNext();
      });
  }

  function startDownload() {
    if (!isReady || sessionIds.length === 0) return;

    isReady = false;
    isAnalyzing = true;
    isPaused = false;
    
    downloadBtn.disabled = true;
    phaseEl.textContent = "downloading";
    updateControlButtons();

    // Download from all sessions
    var downloadPromises = sessionIds.map(function (sid) {
      return fetch("/api/session/" + sid + "/download", { method: "POST" })
        .then(function (resp) {
          if (!resp.ok) return resp.json().then(function (d) { throw new Error(d.error); });
          return resp.json();
        });
    });

    Promise.all(downloadPromises)
      .then(function () {
        // Poll all sessions for completion
        startMultiSessionPolling();
      })
      .catch(function (err) {
        showError(err.message || "Failed to start download");
        isReady = true;
        isAnalyzing = false;
        
        downloadBtn.disabled = false;
        updateControlButtons();
      });
  }

  function startMultiSessionPolling() {
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = setInterval(pollAllSessions, 800);
    pollAllSessions();
  }

  function pollAllSessions() {
    var promises = sessionIds.map(function (sid) {
      return fetch("/api/session/" + sid).then(function (r) { return r.json(); });
    });

    Promise.all(promises)
      .then(function (sessions) {
        // Check if all sessions are done or canceled
        var allDone = sessions.every(function (s) {
          return s.status === "done" || s.status === "error" || s.status === "canceled";
        });

        // Check if any session is paused
        var anyPaused = sessions.some(function (s) {
          return s.status === "paused";
        });

        // Update progress from all sessions
        var totals = { searched: 0, selected: 0, queued: 0, skipped: 0, needs_review: 0, not_found: 0, total: 0 };
        var allTracks = [];
        sessions.forEach(function (s) {
          totals.searched += s.progress.searched;
          totals.selected += s.progress.selected;
          totals.queued += s.progress.queued;
          totals.skipped += s.progress.skipped;
          totals.needs_review += s.progress.needs_review;
          totals.not_found += s.progress.not_found;
          totals.total += s.progress.total;
          if (s.tracks) {
            for (var i = 0; i < s.tracks.length; i++) {
              s.tracks[i]._originalIndex = i;
              s.tracks[i]._sessionId = s.id;
              allTracks.push(s.tracks[i]);
            }
          }
        });

        countSearched.textContent = totals.searched;
        countSelected.textContent = totals.selected;
        countQueued.textContent = totals.queued;
        countSkipped.textContent = totals.skipped;
        countReview.textContent = totals.needs_review;
        countNotFound.textContent = totals.not_found;
        countTotal.textContent = totals.total;

        currentTracks = allTracks;
        totalProgress = totals;
        renderTracks(sortTracks(currentTracks), null, false);

        isPaused = anyPaused;
        updateControlButtons();

        if (anyPaused) {
          phaseEl.textContent = "downloading (paused)";
          phaseEl.classList.add("paused");
        } else {
          phaseEl.classList.remove("paused");
        }

        if (allDone) {
          clearInterval(pollTimer);
          pollTimer = null;
          phaseEl.textContent = "done";
          phaseEl.classList.remove("paused");
          downloadBtn.classList.remove("active");
          isAnalyzing = false;
          isPaused = false;
          
          addBtn.disabled = false;
          updateControlButtons();

          // Check for errors
          var errors = sessions.filter(function (s) { return s.status === "error"; });
          if (errors.length > 0) {
            showError("Some downloads failed");
          }
        } else if (!anyPaused) {
          phaseEl.textContent = "downloading";
        }
      })
      .catch(function () {
        clearInterval(pollTimer);
        pollTimer = null;
        isAnalyzing = false;
        isPaused = false;
        
        addBtn.disabled = false;
        updateControlButtons();
      });
  }

  function pauseAllSessions() {
    if (sessionIds.length === 0) return;

    var promises = sessionIds.map(function (sid) {
      return fetch("/api/session/" + sid + "/pause", { method: "POST" })
        .then(function (resp) {
          if (!resp.ok) return resp.json().then(function (d) { throw new Error(d.error); });
          return resp.json();
        })
        .catch(function () {}); // Ignore errors for already paused/completed sessions
    });

    Promise.all(promises).then(function () {
      isPaused = true;
      updateControlButtons();
    });
  }

  function resumeAllSessions() {
    if (sessionIds.length === 0) return;

    var promises = sessionIds.map(function (sid) {
      return fetch("/api/session/" + sid + "/resume", { method: "POST" })
        .then(function (resp) {
          if (!resp.ok) return resp.json().then(function (d) { throw new Error(d.error); });
          return resp.json();
        })
        .catch(function () {}); // Ignore errors for non-paused sessions
    });

    Promise.all(promises).then(function () {
      isPaused = false;
      updateControlButtons();
    });
  }

  function cancelAllSessions() {
    if (sessionIds.length === 0) return;

    var promises = sessionIds.map(function (sid) {
      return fetch("/api/session/" + sid + "/cancel", { method: "POST" })
        .then(function (resp) {
          if (!resp.ok) return resp.json().then(function (d) { throw new Error(d.error); });
          return resp.json();
        })
        .catch(function () {}); // Ignore errors for already completed sessions
    });

    Promise.all(promises).then(function () {
      // Stop polling and reset state
      if (pollTimer) {
        clearInterval(pollTimer);
        pollTimer = null;
      }
      isAnalyzing = false;
      isReady = false;
      isPaused = false;
      phaseEl.textContent = "canceled";
      phaseEl.classList.remove("paused");
      
      downloadBtn.classList.remove("active");
      downloadBtn.disabled = true;
      addBtn.disabled = false;
      updateControlButtons();
    });
  }

  function updateControlButtons() {
    // Update analyze button text based on state
    if (isPaused) {
      analyzeBtn.textContent = "resume";
      cancelBtn.classList.add("active");
    } else if (isAnalyzing) {
      analyzeBtn.textContent = "pause";
      cancelBtn.classList.add("active");
    } else {
      analyzeBtn.textContent = "analyze";
      cancelBtn.classList.remove("active");
    }
  }

  function startPolling() {
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = setInterval(pollSession, 800);
    pollSession();
  }

  function pollSession() {
    if (!currentSessionId) return;

    fetch("/api/session/" + currentSessionId)
      .then(function (resp) { return resp.json(); })
      .then(function (session) {
        // Handle paused state
        if (session.status === "paused") {
          isPaused = true;
          updateControlButtons();
          phaseEl.classList.add("paused");
          var prefix = urlQueue.length > 1 ? "(" + (syncIndex + 1) + "/" + urlQueue.length + ") " : "";
          phaseEl.textContent = prefix + "paused";
          return;
        } else {
          isPaused = false;
          phaseEl.classList.remove("paused");
          updateControlButtons();
        }

        renderSession(session, false);
        if (session.status === "ready") {
          clearInterval(pollTimer);
          pollTimer = null;

          // Accumulate tracks from this session
          for (var i = 0; i < session.tracks.length; i++) {
            session.tracks[i]._originalIndex = i;
            session.tracks[i]._sessionId = session.id;
            currentTracks.push(session.tracks[i]);
          }

          // Accumulate progress stats
          totalProgress.searched += session.progress.searched;
          totalProgress.selected += session.progress.selected;
          totalProgress.queued += session.progress.queued;
          totalProgress.skipped += session.progress.skipped;
          totalProgress.needs_review += session.progress.needs_review;
          totalProgress.not_found += session.progress.not_found;
          totalProgress.total += session.progress.total;

          syncIndex++;
          if (syncIndex < urlQueue.length) {
            // More URLs to analyze - continue
            analyzeNext();
          } else {
            // All done - show accumulated results
            isReady = true;
            isAnalyzing = false;
            isPaused = false;
            
            downloadBtn.classList.add("active");
            downloadBtn.disabled = false;
            addBtn.disabled = false;
            phaseEl.textContent = "ready";
            updateControlButtons();
            renderSession({ tracks: currentTracks, progress: totalProgress, status: "ready", id: null }, true);
            renderTracks(sortTracks(currentTracks), null, true);
          }
        } else if (session.status === "done" || session.status === "error" || session.status === "canceled") {
          clearInterval(pollTimer);
          pollTimer = null;
          isReady = false;
          isPaused = false;
          downloadBtn.classList.remove("active");
          updateControlButtons();
          if (session.status === "error") {
            showError(session.error || "Failed for: " + (urlQueue[syncIndex] ? urlQueue[syncIndex].url : currentSessionId));
          }
          if (session.status === "canceled") {
            phaseEl.textContent = "canceled";
          }
          syncIndex++;
          if (isAnalyzing && syncIndex < urlQueue.length) {
            analyzeNext();
          } else {
            isAnalyzing = false;
            addBtn.disabled = false;
            updateControlButtons();
          }
        }
      })
      .catch(function () {
        clearInterval(pollTimer);
        pollTimer = null;
        syncIndex++;
        isPaused = false;
        if (isAnalyzing && syncIndex < urlQueue.length) {
          analyzeNext();
        } else {
          isAnalyzing = false;
          addBtn.disabled = false;
          updateControlButtons();
        }
      });
  }

  function renderSession(session, isFinal) {
    var prefix = urlQueue.length > 1 ? "(" + (syncIndex + 1) + "/" + urlQueue.length + ") " : "";
    phaseEl.textContent = prefix + session.status;

    // For in-progress sessions, show current session stats
    // For final render, show accumulated totals
    if (isFinal) {
      countSearched.textContent = totalProgress.searched;
      countSelected.textContent = totalProgress.selected;
      countQueued.textContent = totalProgress.queued;
      countSkipped.textContent = totalProgress.skipped;
      countReview.textContent = totalProgress.needs_review;
      countNotFound.textContent = totalProgress.not_found;
      countTotal.textContent = totalProgress.total;
    } else {
      // Show current session progress + accumulated from previous sessions
      countSearched.textContent = totalProgress.searched + session.progress.searched;
      countSelected.textContent = totalProgress.selected + session.progress.selected;
      countQueued.textContent = totalProgress.queued + session.progress.queued;
      countSkipped.textContent = totalProgress.skipped + session.progress.skipped;
      countReview.textContent = totalProgress.needs_review + session.progress.needs_review;
      countNotFound.textContent = totalProgress.not_found + session.progress.not_found;
      countTotal.textContent = totalProgress.total + session.progress.total;
    }

    if (session.tracks && session.tracks.length > 0) {
      trackContainer.classList.add("active");
      // Preserve original indices and session ID for API calls
      // Only set if not already set (for accumulated tracks from multiple sessions)
      for (var i = 0; i < session.tracks.length; i++) {
        if (session.tracks[i]._originalIndex === undefined) {
          session.tracks[i]._originalIndex = i;
        }
        if (!session.tracks[i]._sessionId && session.id) {
          session.tracks[i]._sessionId = session.id;
        }
      }
      // During analysis, show only current session tracks
      // Final accumulated tracks are built when session reaches ready
      renderTracks(sortTracks(session.tracks), session.id, session.status === "ready");
    }
  }

  function renderTracks(tracks, sid, editable) {
    clearElement(trackBody);
    for (var i = 0; i < tracks.length; i++) {
      var tr = document.createElement("tr");
      var t = tracks[i];
      var trackSid = t._sessionId || sid;

      // Checkbox column
      var tdSelect = document.createElement("td");
      tdSelect.className = "col-select";
      var checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.className = "track-select";
      checkbox.checked = t.selected;
      checkbox.dataset.index = t._originalIndex !== undefined ? t._originalIndex : i;
      checkbox.dataset.sid = trackSid;
      checkbox.disabled = !editable || t.status === "queued";
      checkbox.addEventListener("change", function () {
        toggleTrackSelection(this.dataset.sid, parseInt(this.dataset.index, 10), this.checked);
      });
      tdSelect.appendChild(checkbox);

      var tdTitle = document.createElement("td");
      tdTitle.className = "track-title";
      if (t.parsed_artist) {
        var artistSpan = document.createElement("span");
        artistSpan.className = "track-artist";
        artistSpan.textContent = t.parsed_artist;
        tdTitle.appendChild(artistSpan);
      }
      var songSpan = document.createElement("span");
      songSpan.className = "track-song";
      songSpan.textContent = t.parsed_song || t.youtube_title;
      tdTitle.appendChild(songSpan);
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

        // Add search button for needs_review and skipped tracks
        if (editable && (t.status === "needs_review" || t.status === "skipped")) {
          var searchBtn = document.createElement("button");
          searchBtn.className = "search-btn";
          searchBtn.textContent = "\u270E"; // pencil icon
          searchBtn.title = "Search for different match";
          searchBtn.dataset.index = t._originalIndex !== undefined ? t._originalIndex : i;
          searchBtn.dataset.sid = trackSid;
          searchBtn.addEventListener("click", function () {
            showSearchInput(this.parentElement, this.dataset.sid, parseInt(this.dataset.index, 10));
          });
          tdResult.appendChild(searchBtn);
        }
      } else if (editable && t.status === "not_found") {
        // Show search input for not found tracks
        createSearchInput(tdResult, trackSid, t._originalIndex !== undefined ? t._originalIndex : i);
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
        totalProgress.selected += selected ? 1 : -1;
        countSelected.textContent = totalProgress.selected;
        // Update the track in currentTracks
        for (var i = 0; i < currentTracks.length; i++) {
          if (currentTracks[i]._sessionId === sid && currentTracks[i]._originalIndex === index) {
            currentTracks[i].selected = selected;
            break;
          }
        }
        updateSelectAllState();
      })
      .catch(function (err) {
        showError(err.message || "Failed to update selection");
        // Revert checkbox
        var checkbox = trackBody.querySelector('input[data-sid="' + sid + '"][data-index="' + index + '"]');
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
        return resp.json();
      })
      .then(function (data) {
        // Update the specific track in currentTracks with new match data
        for (var i = 0; i < currentTracks.length; i++) {
          if (currentTracks[i]._sessionId === sid && currentTracks[i]._originalIndex === index) {
            currentTracks[i].deezer_match = data.deezer_match;
            currentTracks[i].confidence = data.confidence;
            // Update status based on confidence
            if (data.deezer_match) {
              currentTracks[i].status = data.confidence >= 70 ? "found" : "needs_review";
              currentTracks[i].selected = data.confidence >= 70;
            }
            break;
          }
        }
        // Re-render with updated tracks
        renderTracks(sortTracks(currentTracks), null, isReady);
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
