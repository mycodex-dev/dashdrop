/* Shared utilities for Dashdrop */

let maxUploadBytes = 5242880;

async function loadConfig() {
  try {
    const res = await fetch("/api/config");
    if (res.ok) {
      const data = await res.json();
      maxUploadBytes = data.max_upload_bytes || maxUploadBytes;
    }
  } catch (_) {
    /* use default */
  }
}

function formatBytes(bytes) {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / (1024 * 1024)).toFixed(1) + " MB";
}

function timeAgo(dateStr) {
  const date = new Date(dateStr);
  if (!Number.isFinite(date.getTime()) || date.getFullYear() < 1970) {
    return "";
  }
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return minutes + " minute" + (minutes === 1 ? "" : "s") + " ago";
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return hours + " hour" + (hours === 1 ? "" : "s") + " ago";
  const days = Math.floor(hours / 24);
  if (days < 30) return days + " day" + (days === 1 ? "" : "s") + " ago";
  const months = Math.floor(days / 30);
  if (months < 12) return months + " month" + (months === 1 ? "" : "s") + " ago";
  const years = Math.floor(months / 12);
  return years + " year" + (years === 1 ? "" : "s") + " ago";
}

function isValidTimestamp(dateStr) {
  if (!dateStr) return false;
  const date = new Date(dateStr);
  return Number.isFinite(date.getTime()) && date.getFullYear() >= 1970;
}

function dashboardDateLabel(d) {
  if (isValidTimestamp(d.updated_at) && d.updated_at !== d.created_at) {
    const label = timeAgo(d.updated_at);
    return label ? "Updated " + label : timeAgo(d.created_at);
  }
  return timeAgo(d.created_at);
}

function showToast(message, type = "success") {
  let container = document.querySelector(".toast-container");
  if (!container) {
    container = document.createElement("div");
    container.className = "toast-container";
    document.body.appendChild(container);
  }
  const toast = document.createElement("div");
  toast.className = "toast " + type;
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 3000);
}

async function copyToClipboard(text) {
  try {
    await navigator.clipboard.writeText(text);
    showToast("Link copied to clipboard");
    return true;
  } catch (_) {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    document.body.appendChild(ta);
    ta.select();
    try {
      document.execCommand("copy");
      showToast("Link copied to clipboard");
      return true;
    } catch (e) {
      showToast("Failed to copy link", "error");
      return false;
    } finally {
      ta.remove();
    }
  }
}

function validateHtmlFile(file) {
  if (!file) return "No file selected";
  const name = file.name.toLowerCase();
  if (!name.endsWith(".html") && !name.endsWith(".htm")) {
    return "Only .html files are allowed";
  }
  if (file.size > maxUploadBytes) {
    return "File exceeds maximum size of " + formatBytes(maxUploadBytes);
  }
  if (file.size === 0) {
    return "File is empty";
  }
  return null;
}

function canvasToBlob(canvas, quality) {
  return new Promise((resolve, reject) => {
    canvas.toBlob(
      (blob) => (blob ? resolve(blob) : reject(new Error("Failed to create thumbnail"))),
      "image/png",
      quality
    );
  });
}

function createFallbackThumbnail(width, height, filename) {
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");

  ctx.fillStyle = "#e8edf2";
  ctx.fillRect(0, 0, width, height);

  ctx.fillStyle = "#0f766e";
  ctx.fillRect(0, 0, width, 56);

  ctx.fillStyle = "#ffffff";
  ctx.font = "bold 22px system-ui, sans-serif";
  const title = filename.replace(/\.html?$/i, "");
  ctx.fillText(title.length > 40 ? title.slice(0, 37) + "..." : title, 24, 36);

  ctx.fillStyle = "#5a6f82";
  ctx.font = "15px system-ui, sans-serif";
  ctx.fillText("Dashboard preview", 24, Math.round(height / 2));

  return canvasToBlob(canvas, 0.85);
}

async function generateThumbnail(html, filename) {
  const blob = new Blob([html], { type: "text/html" });
  const url = URL.createObjectURL(blob);

  const iframe = document.getElementById("thumb-frame");
  if (!iframe) throw new Error("Thumbnail frame not found");

  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      URL.revokeObjectURL(url);
      reject(new Error("Thumbnail generation timed out"));
    }, 15000);

    iframe.onload = async () => {
      try {
        await new Promise((r) => setTimeout(r, 500));

        const doc = iframe.contentDocument;
        const body = doc.body;
        const htmlEl = doc.documentElement;

        const width = Math.min(Math.max(body.scrollWidth, htmlEl.scrollWidth, 800), 1280);
        const height = Math.min(Math.max(body.scrollHeight, htmlEl.scrollHeight, 600), 800);

        if (typeof html2canvas === "undefined") {
          clearTimeout(timeout);
          URL.revokeObjectURL(url);
          resolve(await createFallbackThumbnail(width, height, filename));
          return;
        }

        try {
          const captured = await html2canvas(body, {
            width: width,
            height: height,
            scale: 1,
            useCORS: true,
            allowTaint: true,
            logging: false,
            backgroundColor: "#ffffff",
          });
          clearTimeout(timeout);
          URL.revokeObjectURL(url);
          resolve(await canvasToBlob(captured, 0.85));
        } catch (captureErr) {
          console.warn("Thumbnail capture failed, using fallback:", captureErr);
          clearTimeout(timeout);
          URL.revokeObjectURL(url);
          resolve(await createFallbackThumbnail(width, height, filename));
        }
      } catch (err) {
        clearTimeout(timeout);
        URL.revokeObjectURL(url);
        try {
          resolve(await createFallbackThumbnail(1280, 800, filename));
        } catch {
          reject(err);
        }
      }
    };

    iframe.onerror = () => {
      clearTimeout(timeout);
      URL.revokeObjectURL(url);
      reject(new Error("Failed to load HTML for thumbnail"));
    };

    iframe.src = url;
  });
}

async function uploadDashboard(htmlFile, onProgress) {
  const html = await htmlFile.text();

  onProgress?.("Generating preview...");
  const thumbBlob = await generateThumbnail(html, htmlFile.name);

  onProgress?.("Uploading...");
  const form = new FormData();
  form.append("html", new Blob([html], { type: "text/html" }), htmlFile.name);
  form.append("thumb", thumbBlob, "thumb.png");

  const res = await fetch("/api/upload", { method: "POST", body: form });
  const data = await res.json();
  if (!res.ok) {
    throw new Error(data.error || "Upload failed");
  }
  return data;
}

async function replaceDashboard(slug, htmlFile, onProgress) {
  const html = await htmlFile.text();

  onProgress?.("Generating preview...");
  const thumbBlob = await generateThumbnail(html, htmlFile.name);

  onProgress?.("Uploading new version...");
  const form = new FormData();
  form.append("html", new Blob([html], { type: "text/html" }), htmlFile.name);
  form.append("thumb", thumbBlob, "thumb.png");

  const res = await fetch("/api/dashboards/" + slug, { method: "PUT", body: form });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || "Failed to upload new version");
  }
  return data;
}

async function fetchDashboards() {
  const res = await fetch("/api/dashboards");
  if (!res.ok) throw new Error("Failed to load dashboards");
  return res.json();
}

async function deleteDashboard(slug) {
  const res = await fetch("/api/dashboards/" + slug, { method: "DELETE" });
  if (!res.ok && res.status !== 204) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || "Failed to delete dashboard");
  }
}

async function checkSlugAvailability(slug, exceptSlug) {
  const params = exceptSlug ? "?except=" + encodeURIComponent(exceptSlug) : "";
  const res = await fetch("/api/slugs/" + encodeURIComponent(slug) + params);
  if (!res.ok) throw new Error("Failed to check slug");
  return res.json();
}

async function updateDashboardMeta(currentSlug, { title, slug }) {
  const res = await fetch("/api/dashboards/" + encodeURIComponent(currentSlug), {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title, slug }),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || "Failed to update dashboard");
  }
  return data;
}

function normalizeSlugInput(value) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9-]+/g, "-")
    .replace(/-{2,}/g, "-")
    .replace(/^-+|-+$/g, "");
}

function publicUrlForSlug(slug) {
  return window.location.origin + "/d/" + slug;
}

function createSlugChecker({ input, statusEl, exceptSlug, onChange }) {
  let timer = null;
  let seq = 0;

  async function run() {
    const raw = input.value;
    const slug = normalizeSlugInput(raw);
    if (slug !== raw) {
      input.value = slug;
    }

    const mySeq = ++seq;
    if (!slug) {
      statusEl.textContent = "Enter a URL slug";
      statusEl.className = "field-hint bad";
      onChange?.({ slug: "", available: false, valid: false });
      return;
    }

    statusEl.textContent = "Checking…";
    statusEl.className = "field-hint";

    try {
      const except = typeof exceptSlug === "function" ? exceptSlug() : exceptSlug;
      const result = await checkSlugAvailability(slug, except);
      if (mySeq !== seq) return;
      if (!result.valid) {
        statusEl.textContent = result.error || "Invalid slug";
        statusEl.className = "field-hint bad";
      } else if (!result.available) {
        statusEl.textContent = "Already taken";
        statusEl.className = "field-hint bad";
      } else {
        statusEl.textContent = "Available";
        statusEl.className = "field-hint ok";
      }
      onChange?.(result);
    } catch (e) {
      if (mySeq !== seq) return;
      statusEl.textContent = e.message || "Could not check slug";
      statusEl.className = "field-hint bad";
      onChange?.({ slug, available: false, valid: false });
    }
  }

  function schedule() {
    clearTimeout(timer);
    timer = setTimeout(run, 300);
  }

  input.addEventListener("input", schedule);
  return { checkNow: run };
}

/* Upload page init */
function initUploadPage() {
  const dropzone = document.getElementById("dropzone");
  const fileInput = document.getElementById("file-input");
  const progressWrap = document.getElementById("progress");
  const progressLabel = document.getElementById("progress-label");
  const successPanel = document.getElementById("success");
  const errorMsg = document.getElementById("error");
  const uploadSection = document.getElementById("upload-section");
  const titleInput = document.getElementById("edit-title");
  const slugInput = document.getElementById("edit-slug");
  const slugStatus = document.getElementById("slug-status");
  const slugPrefix = document.getElementById("slug-prefix");
  const saveMetaBtn = document.getElementById("btn-save-meta");
  const urlInput = document.getElementById("published-url");

  if (!dropzone) return;

  loadConfig();

  let currentSlug = "";
  let currentTitle = "";
  let slugOk = true;

  if (slugPrefix) {
    slugPrefix.textContent = window.location.origin + "/d/";
  }

  function showError(msg) {
    errorMsg.textContent = msg;
    errorMsg.classList.add("active");
  }

  function hideError() {
    errorMsg.classList.remove("active");
  }

  function setProgress(active, label) {
    progressWrap.classList.toggle("active", active);
    if (label) progressLabel.textContent = label;
  }

  function refreshPublishedUrl(slug) {
    const url = publicUrlForSlug(slug);
    urlInput.value = url;
    document.getElementById("btn-view").href = url;
    document.getElementById("btn-copy").onclick = () => copyToClipboard(url);
  }

  function updateSaveEnabled() {
    const title = titleInput.value.trim();
    const slug = normalizeSlugInput(slugInput.value);
    const changed = title !== currentTitle || slug !== currentSlug;
    saveMetaBtn.disabled = !changed || !title || !slugOk;
  }

  const slugChecker = createSlugChecker({
    input: slugInput,
    statusEl: slugStatus,
    exceptSlug: () => currentSlug,
    onChange: (result) => {
      slugOk = !!(result.valid && result.available);
      if (result.slug) {
        refreshPublishedUrl(result.slug);
      }
      updateSaveEnabled();
    },
  });

  titleInput.addEventListener("input", updateSaveEnabled);

  saveMetaBtn.addEventListener("click", async () => {
    const title = titleInput.value.trim();
    const slug = normalizeSlugInput(slugInput.value);
    if (!title || !slugOk) return;

    saveMetaBtn.disabled = true;
    try {
      const result = await updateDashboardMeta(currentSlug, { title, slug });
      currentSlug = result.slug;
      currentTitle = result.title;
      titleInput.value = result.title;
      slugInput.value = result.slug;
      refreshPublishedUrl(result.slug);
      slugStatus.textContent = "Saved";
      slugStatus.className = "field-hint ok";
      showToast("Dashboard updated");
      updateSaveEnabled();
    } catch (e) {
      showToast(e.message || "Failed to save changes", "error");
      updateSaveEnabled();
      slugChecker.checkNow();
    }
  });

  async function handleFile(file) {
    hideError();
    const err = validateHtmlFile(file);
    if (err) {
      showError(err);
      return;
    }

    setProgress(true, "Generating preview...");
    dropzone.style.pointerEvents = "none";

    try {
      const result = await uploadDashboard(file, (label) => {
        progressLabel.textContent = label;
      });

      uploadSection.style.display = "none";
      setProgress(false);
      successPanel.classList.add("active");

      currentSlug = result.slug;
      currentTitle = result.title;
      titleInput.value = result.title;
      slugInput.value = result.slug;
      slugOk = true;
      slugStatus.textContent = "Available";
      slugStatus.className = "field-hint ok";
      refreshPublishedUrl(result.slug);
      updateSaveEnabled();

      document.getElementById("btn-upload-another").onclick = () => location.reload();
    } catch (e) {
      showError(e.message || "Upload failed");
      setProgress(false);
      dropzone.style.pointerEvents = "";
    }
  }

  dropzone.addEventListener("dragover", (e) => {
    e.preventDefault();
    dropzone.classList.add("dragover");
  });

  dropzone.addEventListener("dragleave", () => {
    dropzone.classList.remove("dragover");
  });

  dropzone.addEventListener("drop", (e) => {
    e.preventDefault();
    dropzone.classList.remove("dragover");
    const file = e.dataTransfer.files[0];
    if (file) handleFile(file);
  });

  fileInput.addEventListener("change", () => {
    const file = fileInput.files[0];
    if (file) handleFile(file);
  });
}

/* Library page init */
function initLibraryPage() {
  const grid = document.getElementById("dashboard-grid");
  const emptyState = document.getElementById("empty-state");
  const loading = document.getElementById("loading");
  const countEl = document.getElementById("dashboard-count");
  const replaceInput = document.getElementById("replace-input");

  if (!grid) return;

  loadConfig();

  let replaceSlug = null;

  async function render() {
    try {
      const dashboards = await fetchDashboards();
      loading.style.display = "none";

      countEl.textContent =
        dashboards.length + " dashboard" + (dashboards.length === 1 ? "" : "s");

      if (dashboards.length === 0) {
        emptyState.style.display = "block";
        grid.style.display = "none";
        return;
      }

      emptyState.style.display = "none";
      grid.style.display = "grid";
      grid.innerHTML = "";

      for (const d of dashboards) {
        const card = document.createElement("article");
        card.className = "dashboard-card";
        const dateLabel = dashboardDateLabel(d);

        card.innerHTML =
          '<a href="' +
          d.url +
          '" target="_blank" rel="noopener" class="card-thumb">' +
          '<img src="' +
          d.thumb_url +
          "?t=" +
          Date.now() +
          '" alt="' +
          escapeHtml(d.title) +
          '" loading="lazy" onerror="this.style.display=\'none\';this.nextElementSibling.style.display=\'grid\'">' +
          '<div class="card-thumb-placeholder" style="display:none">📊</div>' +
          "</a>" +
          '<div class="card-body">' +
          '<div class="card-title">' +
          escapeHtml(d.title) +
          "</div>" +
          '<div class="card-meta">' +
          dateLabel +
          "</div>" +
          '<div class="card-actions">' +
          '<button type="button" class="btn btn-secondary btn-sm btn-copy">Copy Link</button>' +
          '<button type="button" class="btn btn-secondary btn-sm btn-edit">Edit</button>' +
          '<button type="button" class="btn btn-secondary btn-sm btn-replace">Upload New Version</button>' +
          '<a href="/api/dashboards/' +
          d.slug +
          '/download" class="btn btn-secondary btn-sm">Download</a>' +
          '<button type="button" class="btn btn-danger btn-sm btn-delete">Delete</button>' +
          "</div>" +
          '<div class="edit-panel">' +
          '<label class="field"><span class="field-label">Name</span>' +
          '<input type="text" class="edit-title" maxlength="120" value="' +
          escapeHtml(d.title) +
          '"></label>' +
          '<label class="field"><span class="field-label">URL slug</span>' +
          '<div class="slug-input-row"><span class="slug-prefix">' +
          escapeHtml(window.location.origin + "/d/") +
          '</span><input type="text" class="edit-slug" maxlength="48" spellcheck="false" value="' +
          escapeHtml(d.slug) +
          '"></div>' +
          '<span class="field-hint edit-slug-status"></span></label>' +
          '<div class="edit-actions">' +
          '<button type="button" class="btn btn-primary btn-sm btn-save-edit" disabled>Save</button>' +
          '<button type="button" class="btn btn-secondary btn-sm btn-cancel-edit">Cancel</button>' +
          "</div></div></div>";

        let cardSlug = d.slug;
        let cardTitle = d.title;
        let cardSlugOk = true;
        const editPanel = card.querySelector(".edit-panel");
        const titleField = card.querySelector(".edit-title");
        const slugField = card.querySelector(".edit-slug");
        const slugStatus = card.querySelector(".edit-slug-status");
        const saveEditBtn = card.querySelector(".btn-save-edit");

        function updateCardSaveEnabled() {
          const title = titleField.value.trim();
          const slug = normalizeSlugInput(slugField.value);
          const changed = title !== cardTitle || slug !== cardSlug;
          saveEditBtn.disabled = !changed || !title || !cardSlugOk;
        }

        const checker = createSlugChecker({
          input: slugField,
          statusEl: slugStatus,
          exceptSlug: () => cardSlug,
          onChange: (result) => {
            cardSlugOk = !!(result.valid && result.available);
            updateCardSaveEnabled();
          },
        });

        titleField.addEventListener("input", updateCardSaveEnabled);

        card.querySelector(".btn-edit").addEventListener("click", () => {
          editPanel.classList.add("active");
          titleField.value = cardTitle;
          slugField.value = cardSlug;
          cardSlugOk = true;
          slugStatus.textContent = "";
          slugStatus.className = "field-hint edit-slug-status";
          updateCardSaveEnabled();
          titleField.focus();
        });

        card.querySelector(".btn-cancel-edit").addEventListener("click", () => {
          editPanel.classList.remove("active");
          titleField.value = cardTitle;
          slugField.value = cardSlug;
        });

        saveEditBtn.addEventListener("click", async () => {
          const title = titleField.value.trim();
          const slug = normalizeSlugInput(slugField.value);
          if (!title || !cardSlugOk) return;
          saveEditBtn.disabled = true;
          try {
            await updateDashboardMeta(cardSlug, { title, slug });
            showToast("Dashboard updated");
            render();
          } catch (e) {
            showToast(e.message || "Failed to update", "error");
            updateCardSaveEnabled();
            checker.checkNow();
          }
        });

        card.querySelector(".btn-copy").addEventListener("click", () =>
          copyToClipboard(window.location.origin + "/d/" + cardSlug)
        );

        card.querySelector(".btn-replace").addEventListener("click", () => {
          replaceSlug = d.slug;
          replaceInput.value = "";
          replaceInput.click();
        });

        card.querySelector(".btn-delete").addEventListener("click", async () => {
          if (!confirm('Delete "' + d.title + '"? This cannot be undone.')) return;
          try {
            await deleteDashboard(d.slug);
            showToast("Dashboard deleted");
            render();
          } catch (e) {
            showToast(e.message, "error");
          }
        });

        grid.appendChild(card);
      }
    } catch (e) {
      loading.textContent = "Failed to load dashboards";
      showToast(e.message, "error");
    }
  }

  if (replaceInput) {
    replaceInput.addEventListener("change", async () => {
      const file = replaceInput.files[0];
      const slug = replaceSlug;
      replaceSlug = null;
      replaceInput.value = "";
      if (!file || !slug) return;

      const err = validateHtmlFile(file);
      if (err) {
        showToast(err, "error");
        return;
      }

      try {
        showToast("Uploading new version...");
        await replaceDashboard(slug, file);
        showToast("New version published");
        render();
      } catch (e) {
        showToast(e.message || "Failed to upload new version", "error");
      }
    });
  }

  render();
}

function escapeHtml(str) {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

document.addEventListener("DOMContentLoaded", () => {
  initUploadPage();
  initLibraryPage();
});
