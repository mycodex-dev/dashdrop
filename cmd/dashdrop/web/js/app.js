/* Shared utilities for Dashdrop */

let maxUploadBytes = 5242880;
let publicPathPrefix = "/d";

async function loadConfig() {
  try {
    const res = await fetch("/api/config");
    if (res.ok) {
      const data = await res.json();
      maxUploadBytes = data.max_upload_bytes || maxUploadBytes;
      if (typeof data.public_path_prefix === "string" && data.public_path_prefix.startsWith("/")) {
        publicPathPrefix = data.public_path_prefix.replace(/\/+$/, "") || "/d";
      }
    }
  } catch (_) {
    /* use default */
  }
}

function publicPathForSlug(slug) {
  return publicPathPrefix + "/" + slug;
}

function publicUrlForSlug(slug) {
  return window.location.origin + publicPathForSlug(slug);
}

function publicOriginPrefix() {
  return window.location.origin + publicPathPrefix + "/";
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
  if (isValidTimestamp(d.updated_at)) {
    const label = timeAgo(d.updated_at);
    return label ? "Updated " + label : "";
  }
  if (isValidTimestamp(d.created_at)) {
    const label = timeAgo(d.created_at);
    return label ? "Updated " + label : "";
  }
  return "";
}

function expiresDateInputValue(expiresAt) {
  if (!isValidTimestamp(expiresAt)) return "";
  const d = new Date(expiresAt);
  const y = d.getUTCFullYear();
  const m = String(d.getUTCMonth() + 1).padStart(2, "0");
  const day = String(d.getUTCDate()).padStart(2, "0");
  return y + "-" + m + "-" + day;
}

function expiresLabel(expiresAt) {
  if (!isValidTimestamp(expiresAt)) return "";
  const d = new Date(expiresAt);
  const formatted = d.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    timeZone: "UTC",
  });
  if (d.getTime() < Date.now()) {
    return "Expired " + formatted;
  }
  return "Expires " + formatted;
}

function expiresEqual(a, b) {
  return expiresDateInputValue(a) === expiresDateInputValue(b);
}

function utcDatePlusDays(days) {
  const now = new Date();
  const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  d.setUTCDate(d.getUTCDate() + days);
  const y = d.getUTCFullYear();
  const m = String(d.getUTCMonth() + 1).padStart(2, "0");
  const day = String(d.getUTCDate()).padStart(2, "0");
  return y + "-" + m + "-" + day;
}

const EXPIRE_PRESETS = [
  { id: "never", label: "Never", days: null },
  { id: "7d", label: "1 week", days: 7 },
  { id: "14d", label: "2 weeks", days: 14 },
  { id: "30d", label: "1 month", days: 30 },
  { id: "custom", label: "Custom", days: undefined },
];

function createExpiresPicker(container, { onChange } = {}) {
  let mode = "never";
  let customDate = "";

  container.classList.add("expires-picker");
  container.innerHTML =
    '<div class="expires-presets" role="group" aria-label="Expiration timeframe"></div>' +
    '<div class="expires-custom">' +
    '<input type="date" class="expires-date" aria-label="Custom expiration date">' +
    "</div>";

  const presetsEl = container.querySelector(".expires-presets");
  const customWrap = container.querySelector(".expires-custom");
  const dateInput = container.querySelector(".expires-date");

  presetsEl.innerHTML = EXPIRE_PRESETS.map(
    (p) =>
      '<button type="button" class="expires-preset" data-preset="' +
      p.id +
      '">' +
      p.label +
      "</button>"
  ).join("");

  function activePresetId() {
    if (mode === "never") return "never";
    if (mode === "custom") return "custom";
    return mode;
  }

  function getValue() {
    if (mode === "never") return "";
    if (mode === "custom") return customDate || "";
    const preset = EXPIRE_PRESETS.find((p) => p.id === mode);
    if (!preset || preset.days == null) return "";
    return utcDatePlusDays(preset.days);
  }

  function render(notify) {
    const active = activePresetId();
    presetsEl.querySelectorAll(".expires-preset").forEach((btn) => {
      btn.classList.toggle("active", btn.dataset.preset === active);
    });
    customWrap.classList.toggle("visible", mode === "custom");
    if (mode === "custom") {
      dateInput.value = customDate;
    }
    if (notify) onChange?.(getValue());
  }

  function setValue(raw) {
    const value = typeof raw === "string" ? raw : expiresDateInputValue(raw);
    if (!value) {
      mode = "never";
      customDate = "";
      render(false);
      return;
    }
    const match = EXPIRE_PRESETS.find(
      (p) => typeof p.days === "number" && utcDatePlusDays(p.days) === value
    );
    if (match) {
      mode = match.id;
      customDate = value;
    } else {
      mode = "custom";
      customDate = value;
    }
    render(false);
  }

  presetsEl.addEventListener("click", (e) => {
    const btn = e.target.closest(".expires-preset");
    if (!btn) return;
    const id = btn.dataset.preset;
    if (id === "custom") {
      mode = "custom";
      if (!customDate) {
        customDate = utcDatePlusDays(7);
      }
      render(true);
      dateInput.focus();
      return;
    }
    mode = id;
    if (id !== "never") {
      const preset = EXPIRE_PRESETS.find((p) => p.id === id);
      if (preset && typeof preset.days === "number") {
        customDate = utcDatePlusDays(preset.days);
      }
    } else {
      customDate = "";
    }
    render(true);
  });

  dateInput.addEventListener("change", () => {
    customDate = dateInput.value || "";
    mode = customDate ? "custom" : "never";
    render(true);
  });

  dateInput.addEventListener("input", () => {
    customDate = dateInput.value || "";
    mode = customDate ? "custom" : "never";
    render(true);
  });

  render(false);

  return { getValue, setValue };
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

async function uploadDashboard(htmlFile, onProgress) {
  const html = await htmlFile.text();

  onProgress?.("Uploading...");
  const form = new FormData();
  form.append("html", new Blob([html], { type: "text/html" }), htmlFile.name);

  const res = await fetch("/api/upload", { method: "POST", body: form });
  const data = await res.json();
  if (!res.ok) {
    throw new Error(data.error || "Upload failed");
  }
  return data;
}

async function replaceDashboard(slug, htmlFile, onProgress) {
  const html = await htmlFile.text();

  onProgress?.("Uploading new version...");
  const form = new FormData();
  form.append("html", new Blob([html], { type: "text/html" }), htmlFile.name);

  const res = await fetch("/api/dashboards/" + slug, { method: "PUT", body: form });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || "Failed to upload new version");
  }
  return data;
}

async function fetchDashboards({ archived } = {}) {
  const url = archived ? "/api/dashboards?archived=1" : "/api/dashboards";
  const res = await fetch(url);
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

async function setDashboardArchived(slug, archived) {
  const res = await fetch("/api/dashboards/" + encodeURIComponent(slug), {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ archived }),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || (archived ? "Failed to archive dashboard" : "Failed to restore dashboard"));
  }
  return data;
}

async function checkSlugAvailability(slug, exceptSlug) {
  const params = exceptSlug ? "?except=" + encodeURIComponent(exceptSlug) : "";
  const res = await fetch("/api/slugs/" + encodeURIComponent(slug) + params);
  if (!res.ok) throw new Error("Failed to check slug");
  return res.json();
}

async function updateDashboardMeta(currentSlug, { title, slug, tags, expiresAt }) {
  const body = { title, slug };
  if (tags !== undefined) {
    body.tags = tags;
  }
  if (expiresAt !== undefined) {
    body.expires_at = expiresAt || "";
  }
  const res = await fetch("/api/dashboards/" + encodeURIComponent(currentSlug), {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
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

const MAX_TAGS = 10;
const MAX_TAG_LEN = 32;

function normalizeTagInput(value) {
  return value
    .toLowerCase()
    .trim()
    .replace(/\s+/g, "-")
    .replace(/[^a-z0-9-_]+/g, "")
    .replace(/^[-_]+|[-_]+$/g, "")
    .slice(0, MAX_TAG_LEN);
}

function tagsEqual(a, b) {
  const left = Array.isArray(a) ? a : [];
  const right = Array.isArray(b) ? b : [];
  if (left.length !== right.length) return false;
  for (let i = 0; i < left.length; i++) {
    if (left[i] !== right[i]) return false;
  }
  return true;
}

  function renderTagChips(tags, { filterable } = {}) {
  const list = Array.isArray(tags) ? tags : [];
  return (
    '<div class="card-tags">' +
    list
      .map((tag) => {
        const label = escapeHtml(tag);
        if (filterable) {
          return (
            '<button type="button" class="tag-chip tag-chip-btn" data-tag="' +
            label +
            '">' +
            label +
            "</button>"
          );
        }
        return '<span class="tag-chip">' + label + "</span>";
      })
      .join("") +
    "</div>"
  );
}

function createTagInput(container, { onChange } = {}) {
  let tags = [];

  container.classList.add("tag-input");
  container.innerHTML =
    '<div class="tag-input-chips"></div>' +
    '<input type="text" class="tag-input-field" maxlength="' +
    MAX_TAG_LEN +
    '" autocomplete="off" spellcheck="false" aria-label="Add tag" placeholder="Add a tag">';

  const chipsEl = container.querySelector(".tag-input-chips");
  const input = container.querySelector(".tag-input-field");

  function render(notify) {
    chipsEl.innerHTML = tags
      .map(
        (tag, i) =>
          '<span class="tag-chip">' +
          escapeHtml(tag) +
          '<button type="button" class="tag-chip-remove" data-index="' +
          i +
          '" aria-label="Remove ' +
          escapeHtml(tag) +
          '">×</button></span>'
      )
      .join("");
    input.placeholder = tags.length === 0 ? container.dataset.placeholder || "Add a tag" : "";
    input.disabled = tags.length >= MAX_TAGS;
    if (notify) onChange?.(tags.slice());
  }

  function addTag(raw) {
    const tag = normalizeTagInput(raw);
    if (!tag || tags.includes(tag) || tags.length >= MAX_TAGS) {
      input.value = "";
      return;
    }
    tags.push(tag);
    input.value = "";
    render(true);
  }

  function removeAt(index) {
    tags.splice(index, 1);
    render(true);
    input.focus();
  }

  chipsEl.addEventListener("click", (e) => {
    const btn = e.target.closest(".tag-chip-remove");
    if (!btn) return;
    removeAt(Number(btn.dataset.index));
  });

  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === "," || e.key === "Tab") {
      if (input.value.trim()) {
        e.preventDefault();
        addTag(input.value);
      }
    } else if (e.key === "Backspace" && !input.value && tags.length > 0) {
      removeAt(tags.length - 1);
    }
  });

  input.addEventListener("blur", () => {
    if (input.value.trim()) addTag(input.value);
  });

  container.addEventListener("click", () => input.focus());

  render(false);

  return {
    getTags: () => tags.slice(),
    setTags: (next) => {
      tags = Array.isArray(next) ? next.map(normalizeTagInput).filter(Boolean) : [];
      const seen = new Set();
      tags = tags.filter((t) => {
        if (seen.has(t) || t.length > MAX_TAG_LEN) return false;
        seen.add(t);
        return true;
      }).slice(0, MAX_TAGS);
      input.value = "";
      render(false);
    },
  };
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
async function initUploadPage() {
  const dropzone = document.getElementById("dropzone");
  const fileInput = document.getElementById("file-input");
  const progressWrap = document.getElementById("progress");
  const progressLabel = document.getElementById("progress-label");
  const successPanel = document.getElementById("success");
  const errorMsg = document.getElementById("error");
  const uploadSection = document.getElementById("upload-section");
  const titleInput = document.getElementById("edit-title");
  const titleStatus = document.getElementById("title-status");
  const slugInput = document.getElementById("edit-slug");
  const slugStatus = document.getElementById("slug-status");
  const slugPrefix = document.getElementById("slug-prefix");
  const copyBtn = document.getElementById("btn-copy");
  const viewBtn = document.getElementById("btn-view");
  const tagsContainer = document.getElementById("edit-tags");
  const tagsStatus = document.getElementById("tags-status");
  const expiresPickerEl = document.getElementById("edit-expires-picker");
  const expiresStatus = document.getElementById("expires-status");
  const browseLibraryBtn = document.getElementById("btn-browse-library");

  if (!dropzone) return;

  await loadConfig();

  let currentSlug = "";
  let currentTitle = "";
  let currentTags = [];
  let currentExpires = "";
  let slugOk = true;
  let savingMeta = false;
  let metaSaveTimer = null;
  let slugSaveTimer = null;
  let tagInput = null;
  let expiresPicker = null;

  function draftSlug() {
    return normalizeSlugInput(slugInput.value);
  }

  tagInput = tagsContainer
    ? createTagInput(tagsContainer, {
        onChange: () => scheduleMetaSave("tags"),
      })
    : null;

  expiresPicker = expiresPickerEl
    ? createExpiresPicker(expiresPickerEl, {
        onChange: () => scheduleMetaSave("expires"),
      })
    : null;

  if (slugPrefix) {
    slugPrefix.textContent = publicOriginPrefix();
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

  function setLiveLinks(slug) {
    if (viewBtn) viewBtn.href = publicUrlForSlug(slug);
  }

  function setHint(el, text, kind) {
    if (!el) return;
    el.textContent = text;
    el.className = "field-hint" + (kind ? " " + kind : "");
  }

  function setTagsStatus(text, kind) {
    setHint(tagsStatus, text, kind);
  }

  function setSlugStatus(text, kind) {
    setHint(slugStatus, text, kind);
  }

  function setTitleStatus(text, kind) {
    setHint(titleStatus, text, kind);
  }

  function setExpiresStatus(text, kind) {
    setHint(expiresStatus, text, kind);
  }

  async function saveMeta({ quiet, slugOnly, reason } = {}) {
    const title = titleInput.value.trim();
    const slug = draftSlug();
    const tags = tagInput ? tagInput.getTags() : currentTags.slice();
    const expires = expiresPicker ? expiresPicker.getValue() : currentExpires;
    if (!currentSlug || savingMeta) return false;

    if (!title) {
      setTitleStatus("Name is required", "bad");
      return false;
    }

    const nextSlug = slugOnly || slug !== currentSlug ? slug : currentSlug;
    if ((slugOnly || slug !== currentSlug) && !slugOk) return false;

    const changed =
      title !== currentTitle ||
      nextSlug !== currentSlug ||
      !tagsEqual(tags, currentTags) ||
      expires !== currentExpires;
    if (!changed) return true;

    savingMeta = true;
    if (slugOnly || nextSlug !== currentSlug) {
      setSlugStatus("Updating link…");
    } else if (reason === "title") {
      setTitleStatus("Saving…");
    } else if (reason === "expires") {
      setExpiresStatus("Saving…");
    } else {
      setTagsStatus("Saving…");
    }

    try {
      const result = await updateDashboardMeta(currentSlug, {
        title,
        slug: nextSlug,
        tags,
        expiresAt: expires,
      });
      currentSlug = result.slug;
      currentTitle = result.title;
      currentTags = Array.isArray(result.tags) ? result.tags : [];
      currentExpires = expiresDateInputValue(result.expires_at);
      titleInput.value = result.title;
      slugInput.value = result.slug;
      tagInput?.setTags(currentTags);
      expiresPicker?.setValue(currentExpires);
      setLiveLinks(result.slug);
      slugOk = true;

      if (slugOnly) {
        setSlugStatus("Link updated", "ok");
      } else if (reason === "title") {
        setTitleStatus("Saved", "ok");
      } else if (reason === "expires") {
        setExpiresStatus(
          currentExpires ? expiresLabel(result.expires_at) || "Saved" : "No expiration",
          "ok"
        );
      } else if (reason === "tags") {
        setTagsStatus("Tags saved", "ok");
      }
      if (!quiet && !slugOnly) showToast("Dashboard updated");
      return true;
    } catch (e) {
      if (slugOnly || nextSlug !== currentSlug) {
        setSlugStatus(e.message || "Failed to update link", "bad");
      } else if (reason === "title") {
        setTitleStatus(e.message || "Failed to save", "bad");
      } else if (reason === "expires") {
        setExpiresStatus(e.message || "Failed to save", "bad");
      } else {
        setTagsStatus(e.message || "Failed to save", "bad");
      }
      if (!quiet) showToast(e.message || "Failed to save changes", "error");
      slugChecker.checkNow();
      return false;
    } finally {
      savingMeta = false;
    }
  }

  function scheduleMetaSave(reason) {
    clearTimeout(metaSaveTimer);
    if (!currentSlug) return;

    if (reason === "title") {
      const title = titleInput.value.trim();
      if (!title) {
        setTitleStatus("Name is required", "bad");
        return;
      }
      if (title === currentTitle) return;
      setTitleStatus("Saving…");
    } else if (reason === "expires") {
      const expires = expiresPicker ? expiresPicker.getValue() : "";
      if (expires === currentExpires) return;
      setExpiresStatus("Saving…");
    } else {
      const tags = tagInput ? tagInput.getTags() : [];
      if (tagsEqual(tags, currentTags)) return;
      setTagsStatus("Saving…");
    }

    metaSaveTimer = setTimeout(() => {
      saveMeta({ quiet: true, reason });
    }, 350);
  }

  function scheduleSlugSave() {
    clearTimeout(slugSaveTimer);
    if (!currentSlug || !slugOk) return;
    const slug = draftSlug();
    if (!slug || slug === currentSlug) return;
    setSlugStatus("Updating link…");
    slugSaveTimer = setTimeout(() => {
      saveMeta({ quiet: true, slugOnly: true });
    }, 450);
  }

  const slugChecker = createSlugChecker({
    input: slugInput,
    statusEl: slugStatus,
    exceptSlug: () => currentSlug,
    onChange: (result) => {
      slugOk = !!(result.valid && result.available);
      const slug = result.slug || draftSlug();
      if (slug && slug === currentSlug && result.valid && result.available) {
        setSlugStatus("Live link", "ok");
        return;
      }
      if (slugOk && slug && slug !== currentSlug) {
        scheduleSlugSave();
      }
    },
  });

  titleInput.addEventListener("input", () => scheduleMetaSave("title"));

  if (copyBtn) {
    copyBtn.addEventListener("click", async () => {
      clearTimeout(slugSaveTimer);
      clearTimeout(metaSaveTimer);
      const slug = draftSlug();
      if (slug && slug !== currentSlug) {
        if (!slugOk) {
          showToast("Fix the URL before copying", "error");
          return;
        }
        const ok = await saveMeta({ quiet: true, slugOnly: true });
        if (!ok) return;
      }
      await copyToClipboard(publicUrlForSlug(currentSlug));
    });
  }

  if (browseLibraryBtn) {
    browseLibraryBtn.addEventListener("click", async (e) => {
      e.preventDefault();
      clearTimeout(metaSaveTimer);
      clearTimeout(slugSaveTimer);
      const field = tagsContainer?.querySelector(".tag-input-field");
      if (field && field.value.trim() && tagInput) {
        field.dispatchEvent(new Event("blur"));
      }
      clearTimeout(metaSaveTimer);
      await saveMeta({ quiet: true });
      window.location.href = "/library";
    });
  }

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
      currentTags = Array.isArray(result.tags) ? result.tags : [];
      currentExpires = expiresDateInputValue(result.expires_at);
      titleInput.value = result.title;
      slugInput.value = result.slug;
      tagInput?.setTags(currentTags);
      expiresPicker?.setValue(currentExpires);
      slugOk = true;
      setSlugStatus("Live link · edit to customize", "ok");
      setTitleStatus("Saved as you type");
      setTagsStatus("Optional · saved as you add them · max 10");
      setExpiresStatus("Optional · archives automatically after this date");
      setLiveLinks(result.slug);

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
async function initLibraryPage() {
  const grid = document.getElementById("dashboard-grid");
  const emptyState = document.getElementById("empty-state");
  const emptyStateTitle = document.getElementById("empty-state-title");
  const emptyStateText = document.getElementById("empty-state-text");
  const emptyStateUpload = document.getElementById("empty-state-upload");
  const btnBackLibrary = document.getElementById("btn-back-library");
  const emptyFilter = document.getElementById("empty-filter");
  const emptyFilterTitle = document.getElementById("empty-filter-title");
  const emptyFilterText = document.getElementById("empty-filter-text");
  const loading = document.getElementById("loading");
  const countEl = document.getElementById("dashboard-count");
  const replaceInput = document.getElementById("replace-input");
  const tagFilterEl = document.getElementById("tag-filter");
  const clearFilterBtn = document.getElementById("btn-clear-filter");
  const toolbar = document.getElementById("library-toolbar");
  const searchInput = document.getElementById("library-search");
  const sortSelect = document.getElementById("library-sort");
  const viewGridBtn = document.getElementById("view-grid");
  const viewListBtn = document.getElementById("view-list");
  const archivedModeBtn = document.getElementById("btn-archived-mode");

  if (!grid) return;

  await loadConfig();

  const params = new URLSearchParams(window.location.search);
  const SORT_OPTIONS = ["newest", "oldest", "name-asc", "name-desc"];
  const VIEW_OPTIONS = ["grid", "list"];

  function storedOr(key, allowed, fallback) {
    try {
      const value = localStorage.getItem(key);
      if (allowed.includes(value)) return value;
    } catch (_) {
      /* ignore */
    }
    return fallback;
  }

  let replaceSlug = null;
  let showArchived =
    params.get("archived") === "1" || params.get("archived") === "true";
  let activeTag = showArchived ? "" : params.get("tag") || "";
  let searchQuery = params.get("q") || "";
  let sortMode = storedOr("dashdrop-library-sort", SORT_OPTIONS, "newest");
  let viewMode = storedOr("dashdrop-library-view", VIEW_OPTIONS, "grid");
  let allDashboards = [];

  if (searchInput && searchQuery) {
    searchInput.value = searchQuery;
  }
  if (sortSelect) {
    sortSelect.value = sortMode;
  }
  applyViewMode(viewMode);
  applyArchivedModeUI();

  function persistPrefs() {
    try {
      localStorage.setItem("dashdrop-library-sort", sortMode);
      localStorage.setItem("dashdrop-library-view", viewMode);
    } catch (_) {
      /* ignore */
    }
  }

  function syncUrl() {
    const url = new URL(window.location.href);
    if (showArchived) url.searchParams.set("archived", "1");
    else url.searchParams.delete("archived");
    if (activeTag && !showArchived) url.searchParams.set("tag", activeTag);
    else url.searchParams.delete("tag");
    if (searchQuery) url.searchParams.set("q", searchQuery);
    else url.searchParams.delete("q");
    window.history.replaceState({}, "", url);
  }

  function applyArchivedModeUI() {
    if (archivedModeBtn) {
      archivedModeBtn.classList.toggle("active", showArchived);
      archivedModeBtn.setAttribute("aria-pressed", showArchived ? "true" : "false");
    }
  }

  function applyViewMode(mode) {
    viewMode = mode === "list" ? "list" : "grid";
    grid.classList.toggle("view-grid", viewMode === "grid");
    grid.classList.toggle("view-list", viewMode === "list");
    if (viewGridBtn) {
      viewGridBtn.classList.toggle("active", viewMode === "grid");
      viewGridBtn.setAttribute("aria-pressed", viewMode === "grid" ? "true" : "false");
    }
    if (viewListBtn) {
      viewListBtn.classList.toggle("active", viewMode === "list");
      viewListBtn.setAttribute("aria-pressed", viewMode === "list" ? "true" : "false");
    }
  }

  function setActiveTag(tag) {
    if (showArchived) return;
    activeTag = tag || "";
    syncUrl();
    renderFilter();
    renderGrid();
  }

  async function setShowArchived(next) {
    showArchived = !!next;
    if (showArchived) activeTag = "";
    applyArchivedModeUI();
    syncUrl();
    await loadAndRender();
  }

  function activityTime(d) {
    const updated = Date.parse(d.updated_at);
    if (Number.isFinite(updated)) return updated;
    const created = Date.parse(d.created_at);
    return Number.isFinite(created) ? created : 0;
  }

  function matchesSearch(d, query) {
    if (!query) return true;
    const haystack = [d.title || "", d.slug || "", ...(showArchived ? [] : d.tags || [])]
      .join(" ")
      .toLowerCase();
    return query
      .toLowerCase()
      .split(/\s+/)
      .filter(Boolean)
      .every((term) => haystack.includes(term));
  }

  function sortDashboards(list) {
    const sorted = list.slice();
    sorted.sort((a, b) => {
      switch (sortMode) {
        case "oldest":
          return activityTime(a) - activityTime(b);
        case "name-asc":
          return (a.title || "").localeCompare(b.title || "", undefined, {
            sensitivity: "base",
          });
        case "name-desc":
          return (b.title || "").localeCompare(a.title || "", undefined, {
            sensitivity: "base",
          });
        case "newest":
        default:
          return activityTime(b) - activityTime(a);
      }
    });
    return sorted;
  }

  function renderFilter() {
    if (!tagFilterEl) return;
    if (showArchived) {
      tagFilterEl.style.display = "none";
      tagFilterEl.innerHTML = "";
      return;
    }
    const tagSet = new Set();
    for (const d of allDashboards) {
      for (const t of d.tags || []) tagSet.add(t);
    }
    const tags = Array.from(tagSet).sort();
    if (tags.length === 0) {
      tagFilterEl.style.display = "none";
      tagFilterEl.innerHTML = "";
      return;
    }

    tagFilterEl.style.display = "flex";
    tagFilterEl.innerHTML =
      '<button type="button" class="tag-filter-chip' +
      (activeTag ? "" : " active") +
      '" data-tag="">All</button>' +
      tags
        .map(
          (tag) =>
            '<button type="button" class="tag-filter-chip' +
            (activeTag === tag ? " active" : "") +
            '" data-tag="' +
            escapeHtml(tag) +
            '">' +
            escapeHtml(tag) +
            "</button>"
        )
        .join("");
  }

  function visibleDashboards() {
    let list = allDashboards;
    if (!showArchived && activeTag) {
      list = list.filter((d) => (d.tags || []).includes(activeTag));
    }
    if (searchQuery) {
      list = list.filter((d) => matchesSearch(d, searchQuery));
    }
    return sortDashboards(list);
  }

  function closeAllMenus(except) {
    grid.querySelectorAll(".card-menu-dropdown.open").forEach((menu) => {
      if (menu === except) return;
      menu.classList.remove("open");
      const cardEl = menu.closest(".dashboard-card");
      if (cardEl) cardEl.classList.remove("is-menu-open");
      const trigger = menu.closest(".card-menu")?.querySelector(".btn-menu");
      if (trigger) trigger.setAttribute("aria-expanded", "false");
    });
  }

  function renderGrid() {
    const dashboards = visibleDashboards();
    const noun = showArchived ? "Archived dashboard" : "Dashboard";
    const parts = [
      dashboards.length + " " + noun + (dashboards.length === 1 ? "" : "s"),
    ];
    if (!showArchived && activeTag) parts.push('tagged "' + activeTag + '"');
    if (searchQuery) parts.push('matching "' + searchQuery + '"');
    countEl.textContent = parts.join(" · ");

    if (toolbar) {
      toolbar.style.display = "flex";
    }

    if (allDashboards.length === 0) {
      emptyState.style.display = "block";
      if (emptyFilter) emptyFilter.style.display = "none";
      grid.style.display = "none";
      if (showArchived) {
        if (emptyStateTitle) emptyStateTitle.textContent = "No archived dashboards";
        if (emptyStateText) emptyStateText.textContent = "Archived dashboards will appear here.";
        if (emptyStateUpload) emptyStateUpload.style.display = "none";
        if (btnBackLibrary) btnBackLibrary.style.display = "inline-flex";
      } else {
        if (emptyStateTitle) emptyStateTitle.textContent = "No dashboards yet";
        if (emptyStateText) emptyStateText.textContent = "Upload your first HTML dashboard.";
        if (emptyStateUpload) emptyStateUpload.style.display = "inline-flex";
        if (btnBackLibrary) btnBackLibrary.style.display = "none";
      }
      return;
    }

    emptyState.style.display = "none";

    if (dashboards.length === 0) {
      if (emptyFilter) {
        emptyFilter.style.display = "block";
        if (emptyFilterTitle) emptyFilterTitle.textContent = "No matching dashboards";
        if (emptyFilterText) {
          if (searchQuery && activeTag && !showArchived) {
            emptyFilterText.textContent =
              'Nothing matches "' + searchQuery + '" with tag "' + activeTag + '".';
          } else if (searchQuery) {
            emptyFilterText.textContent = 'Nothing matches "' + searchQuery + '".';
          } else {
            emptyFilterText.textContent = "No dashboards have this tag.";
          }
        }
      }
      grid.style.display = "none";
      return;
    }

    if (emptyFilter) emptyFilter.style.display = "none";
    grid.style.display = "grid";
    grid.innerHTML = "";

    for (const d of dashboards) {
      const card = document.createElement("article");
      card.className = "dashboard-card";
      const dateLabel = dashboardDateLabel(d);
      const expLabel = expiresLabel(d.expires_at);
      const tags = Array.isArray(d.tags) ? d.tags : [];
      const tagsHtml = showArchived ? "" : renderTagChips(tags, { filterable: true });
      const thumbInner =
        '<img src="' +
        d.thumb_url +
        "?t=" +
        Date.now() +
        '" alt="' +
        escapeHtml(d.title) +
        '" loading="lazy" onerror="this.style.display=\'none\';this.nextElementSibling.style.display=\'grid\'">' +
        '<div class="card-thumb-placeholder" style="display:none">📊</div>';
      const thumbHtml = showArchived
        ? '<div class="card-thumb">' + thumbInner + "</div>"
        : '<a href="' +
          d.url +
          '" target="_blank" rel="noopener" class="card-thumb">' +
          thumbInner +
          "</a>";
      const primaryAction = showArchived
        ? '<button type="button" class="btn btn-primary btn-sm btn-unarchive">Unarchive</button>'
        : '<a href="' +
          d.url +
          '" target="_blank" rel="noopener" class="btn btn-primary btn-sm btn-open">Open</a>';
      const archiveMenuItem = showArchived
        ? ""
        : '<button type="button" class="card-menu-item btn-archive" role="menuitem">Archive</button>';
      const copyMenuItem = showArchived
        ? ""
        : '<button type="button" class="card-menu-item btn-copy" role="menuitem">Copy Link</button>';
      const metaLines =
        (dateLabel ? '<div class="card-meta-line">' + dateLabel + "</div>" : "") +
        (expLabel ? '<div class="card-meta-line">' + escapeHtml(expLabel) + "</div>" : "");

      card.innerHTML =
        thumbHtml +
        '<div class="card-body">' +
        '<div class="card-title">' +
        escapeHtml(d.title) +
        "</div>" +
        '<div class="card-slug">' +
        escapeHtml(publicPathForSlug(d.slug)) +
        "</div>" +
        '<div class="card-meta">' +
        metaLines +
        "</div>" +
        tagsHtml +
        '<div class="card-actions">' +
        primaryAction +
        '<button type="button" class="btn btn-secondary btn-sm btn-edit">Edit</button>' +
        '<div class="card-menu">' +
        '<button type="button" class="btn btn-ghost btn-sm btn-menu" aria-haspopup="true" aria-expanded="false" aria-label="More actions">⋯</button>' +
        '<div class="card-menu-dropdown" role="menu">' +
        copyMenuItem +
        '<a href="/api/dashboards/' +
        d.slug +
        '/download" class="card-menu-item" role="menuitem">Download</a>' +
        '<button type="button" class="card-menu-item btn-replace" role="menuitem">Upload New Version</button>' +
        '<hr class="card-menu-divider">' +
        archiveMenuItem +
        '<button type="button" class="card-menu-item danger btn-delete" role="menuitem">Delete</button>' +
        "</div></div></div>" +
        '<div class="edit-panel">' +
        '<label class="field"><span class="field-label">Name</span>' +
        '<input type="text" class="edit-title" maxlength="120" value="' +
        escapeHtml(d.title) +
        '"></label>' +
        '<label class="field"><span class="field-label">URL slug</span>' +
        '<div class="slug-input-row"><span class="slug-prefix">' +
        escapeHtml(publicOriginPrefix()) +
        '</span><input type="text" class="edit-slug" maxlength="48" spellcheck="false" value="' +
        escapeHtml(d.slug) +
        '"></div>' +
        '<span class="field-hint edit-slug-status"></span></label>' +
        '<div class="field"><span class="field-label">Tags</span>' +
        '<div class="edit-tags tag-input" data-placeholder="Add a tag"></div></div>' +
        '<div class="field"><span class="field-label">Expires</span>' +
        '<div class="edit-expires-picker expires-picker"></div>' +
        '<span class="field-hint">Optional · archives automatically after this date</span></div>' +
        '<div class="edit-actions">' +
        '<button type="button" class="btn btn-primary btn-sm btn-save-edit" disabled>Save</button>' +
        '<button type="button" class="btn btn-secondary btn-sm btn-cancel-edit">Cancel</button>' +
        "</div></div></div>";

      let cardSlug = d.slug;
      let cardTitle = d.title;
      let cardTags = tags.slice();
      let cardExpires = expiresDateInputValue(d.expires_at);
      let cardSlugOk = true;
      const editPanel = card.querySelector(".edit-panel");
      const titleField = card.querySelector(".edit-title");
      const slugField = card.querySelector(".edit-slug");
      const slugStatus = card.querySelector(".edit-slug-status");
      const saveEditBtn = card.querySelector(".btn-save-edit");
      const tagsField = card.querySelector(".edit-tags");
      const expiresPickerField = card.querySelector(".edit-expires-picker");
      const menuBtn = card.querySelector(".btn-menu");
      const menuDropdown = card.querySelector(".card-menu-dropdown");

      const cardTagInput = createTagInput(tagsField, {
        onChange: () => updateCardSaveEnabled(),
      });
      cardTagInput.setTags(cardTags);

      const cardExpiresPicker = createExpiresPicker(expiresPickerField, {
        onChange: () => updateCardSaveEnabled(),
      });
      cardExpiresPicker.setValue(cardExpires);

      function updateCardSaveEnabled() {
        const title = titleField.value.trim();
        const slug = normalizeSlugInput(slugField.value);
        const nextTags = cardTagInput.getTags();
        const nextExpires = cardExpiresPicker.getValue();
        const changed =
          title !== cardTitle ||
          slug !== cardSlug ||
          !tagsEqual(nextTags, cardTags) ||
          nextExpires !== cardExpires;
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

      card.querySelectorAll(".tag-chip-btn").forEach((chip) => {
        chip.addEventListener("click", (e) => {
          e.preventDefault();
          setActiveTag(chip.dataset.tag);
        });
      });

      menuBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        const willOpen = !menuDropdown.classList.contains("open");
        closeAllMenus(willOpen ? menuDropdown : null);
        menuDropdown.classList.toggle("open", willOpen);
        card.classList.toggle("is-menu-open", willOpen);
        menuBtn.setAttribute("aria-expanded", willOpen ? "true" : "false");
      });

      menuDropdown.addEventListener("click", (e) => {
        if (e.target.closest(".card-menu-item")) closeAllMenus();
      });

      card.querySelector(".btn-edit").addEventListener("click", () => {
        closeAllMenus();
        editPanel.classList.add("active");
        titleField.value = cardTitle;
        slugField.value = cardSlug;
        cardTagInput.setTags(cardTags);
        cardExpiresPicker.setValue(cardExpires);
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
        cardTagInput.setTags(cardTags);
        cardExpiresPicker.setValue(cardExpires);
      });

      saveEditBtn.addEventListener("click", async () => {
        const title = titleField.value.trim();
        const slug = normalizeSlugInput(slugField.value);
        const nextTags = cardTagInput.getTags();
        const nextExpires = cardExpiresPicker.getValue();
        if (!title || !cardSlugOk) return;
        saveEditBtn.disabled = true;
        try {
          await updateDashboardMeta(cardSlug, {
            title,
            slug,
            tags: nextTags,
            expiresAt: nextExpires,
          });
          showToast("Dashboard updated");
          await loadAndRender();
        } catch (e) {
          showToast(e.message || "Failed to update", "error");
          updateCardSaveEnabled();
          checker.checkNow();
        }
      });

      const copyBtn = card.querySelector(".btn-copy");
      if (copyBtn) {
        copyBtn.addEventListener("click", () => {
          closeAllMenus();
          copyToClipboard(publicUrlForSlug(cardSlug));
        });
      }

      card.querySelector(".btn-replace").addEventListener("click", () => {
        closeAllMenus();
        replaceSlug = d.slug;
        replaceInput.value = "";
        replaceInput.click();
      });

      const archiveBtn = card.querySelector(".btn-archive");
      if (archiveBtn) {
        archiveBtn.addEventListener("click", async () => {
          closeAllMenus();
          if (
            !confirm(
              'Archive "' +
                d.title +
                '"? It will be hidden from the library and its live URL will stop working. You can unarchive it later.'
            )
          ) {
            return;
          }
          try {
            await setDashboardArchived(d.slug, true);
            showToast("Dashboard archived");
            await loadAndRender();
          } catch (e) {
            showToast(e.message, "error");
          }
        });
      }

      async function unarchiveDashboard() {
        try {
          await setDashboardArchived(d.slug, false);
          showToast("Dashboard unarchived");
          await loadAndRender();
        } catch (e) {
          showToast(e.message, "error");
        }
      }

      const unarchiveBtn = card.querySelector(".btn-unarchive");
      if (unarchiveBtn) {
        unarchiveBtn.addEventListener("click", async () => {
          closeAllMenus();
          await unarchiveDashboard();
        });
      }

      card.querySelector(".btn-delete").addEventListener("click", async () => {
        closeAllMenus();
        if (!confirm('Delete "' + d.title + '"? This cannot be undone.')) return;
        try {
          await deleteDashboard(d.slug);
          showToast("Dashboard deleted");
          await loadAndRender();
        } catch (e) {
          showToast(e.message, "error");
        }
      });

      grid.appendChild(card);
    }
  }

  async function loadAndRender() {
    try {
      allDashboards = await fetchDashboards({ archived: showArchived });
      loading.style.display = "none";
      renderFilter();
      renderGrid();
    } catch (e) {
      loading.textContent = "Failed to load dashboards";
      showToast(e.message, "error");
    }
  }

  if (searchInput) {
    let searchTimer = null;
    searchInput.addEventListener("input", () => {
      clearTimeout(searchTimer);
      searchTimer = setTimeout(() => {
        searchQuery = searchInput.value.trim();
        syncUrl();
        renderGrid();
      }, 150);
    });

    const searchKbd = document.getElementById("library-search-kbd");
    const isMac = /Mac|iPhone|iPad|iPod/i.test(navigator.platform || navigator.userAgent || "");
    if (searchKbd) {
      searchKbd.textContent = isMac ? "⌘K" : "Ctrl K";
    }

    document.addEventListener("keydown", (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        searchInput.focus();
        searchInput.select();
      }
    });
  }

  document.addEventListener("click", (e) => {
    if (!e.target.closest(".card-menu")) closeAllMenus();
  });

  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      closeAllMenus();
      if (searchInput && document.activeElement === searchInput) searchInput.blur();
    }
  });

  if (sortSelect) {
    sortSelect.addEventListener("change", () => {
      sortMode = SORT_OPTIONS.includes(sortSelect.value) ? sortSelect.value : "newest";
      persistPrefs();
      renderGrid();
    });
  }

  if (viewGridBtn) {
    viewGridBtn.addEventListener("click", () => {
      applyViewMode("grid");
      persistPrefs();
    });
  }

  if (viewListBtn) {
    viewListBtn.addEventListener("click", () => {
      applyViewMode("list");
      persistPrefs();
    });
  }

  if (archivedModeBtn) {
    archivedModeBtn.addEventListener("click", () => {
      setShowArchived(!showArchived);
    });
  }

  if (btnBackLibrary) {
    btnBackLibrary.addEventListener("click", () => {
      setShowArchived(false);
    });
  }

  if (tagFilterEl) {
    tagFilterEl.addEventListener("click", (e) => {
      const btn = e.target.closest(".tag-filter-chip");
      if (!btn) return;
      setActiveTag(btn.dataset.tag || "");
    });
  }

  if (clearFilterBtn) {
    clearFilterBtn.addEventListener("click", () => {
      activeTag = "";
      searchQuery = "";
      if (searchInput) searchInput.value = "";
      syncUrl();
      renderFilter();
      renderGrid();
    });
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
        await loadAndRender();
      } catch (e) {
        showToast(e.message || "Failed to upload new version", "error");
      }
    });
  }

  loadAndRender();
}

function escapeHtml(str) {
  const div = document.createElement("div");
  div.textContent = str;
  return div.innerHTML;
}

const DEFAULT_PRIMARY_LIGHT = "#0f766e";
const DEFAULT_PRIMARY_DARK = "#14b8a6";
const HEX_COLOR_RE = /^#[0-9a-fA-F]{6}$/;

function clampByte(n) {
  return Math.max(0, Math.min(255, Math.round(n)));
}

function hexToRgb(hex) {
  const n = parseInt(hex.slice(1), 16);
  return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
}

function rgbToHex(r, g, b) {
  return (
    "#" +
    [r, g, b]
      .map((x) => clampByte(x).toString(16).padStart(2, "0"))
      .join("")
  );
}

function deriveAccentVars(hex) {
  const { r, g, b } = hexToRgb(hex);
  const dark = document.documentElement.getAttribute("data-theme") === "dark";
  const hover = dark
    ? rgbToHex(r + (255 - r) * 0.22, g + (255 - g) * 0.22, b + (255 - b) * 0.22)
    : rgbToHex(r + (255 - r) * 0.18, g + (255 - g) * 0.18, b + (255 - b) * 0.18);
  const softAlpha = dark ? 0.14 : 0.08;
  const glowAlpha = dark ? 0.22 : 0.18;
  return {
    accent: hex.toLowerCase(),
    hover,
    soft: `rgba(${r}, ${g}, ${b}, ${softAlpha})`,
    glow: `rgba(${r}, ${g}, ${b}, ${glowAlpha})`,
    focus: `0 0 0 3px rgba(${r}, ${g}, ${b}, ${glowAlpha})`,
  };
}

function applyAccentColor(hex) {
  if (!HEX_COLOR_RE.test(hex)) return;
  const vars = deriveAccentVars(hex);
  const root = document.documentElement;
  root.style.setProperty("--accent", vars.accent);
  root.style.setProperty("--accent-hover", vars.hover);
  root.style.setProperty("--accent-soft", vars.soft);
  root.style.setProperty("--accent-glow", vars.glow);
  root.style.setProperty("--focus-ring", vars.focus);
  root.style.setProperty("--brand-primary", vars.accent);
}

function clearAccentOverride() {
  const root = document.documentElement;
  ["--accent", "--accent-hover", "--accent-soft", "--accent-glow", "--focus-ring", "--brand-primary"].forEach((prop) => {
    root.style.removeProperty(prop);
  });
}

function setLogoElement(el, logoUrl) {
  if (!el) return;
  if (logoUrl) {
    el.classList.add("has-image");
    el.innerHTML = `<img src="${escapeHtml(logoUrl)}" alt="">`;
  } else {
    el.classList.remove("has-image");
    el.textContent = "⚡";
  }
}

function applyBranding(settings) {
  const name = (settings && settings.app_name) || "Dashdrop";
  const hasLogo = !!(settings && settings.has_logo && settings.logo_url);
  const logoUrl = hasLogo ? settings.logo_url + (settings.logo_url.includes("?") ? "&" : "?") + "t=" + Date.now() : "";
  const primary = settings && HEX_COLOR_RE.test(settings.primary_color || "")
    ? settings.primary_color.toLowerCase()
    : null;

  document.querySelectorAll("[data-brand-name]").forEach((el) => {
    el.textContent = name;
  });
  document.querySelectorAll("[data-brand-logo]").forEach((el) => {
    setLogoElement(el, hasLogo ? logoUrl : "");
  });

  if (primary) {
    applyAccentColor(primary);
  } else {
    clearAccentOverride();
  }

  const path = window.location.pathname;
  if (path === "/" || path === "") {
    document.title = name + " — Publish HTML instantly";
  } else if (path.startsWith("/library")) {
    document.title = "Library — " + name;
  } else if (path.startsWith("/settings")) {
    document.title = "Settings — " + name;
  }

  try {
    localStorage.setItem(
      "dashdrop-brand",
      JSON.stringify({
        app_name: name,
        primary_color: primary || "",
        has_logo: hasLogo,
        logo_url: hasLogo ? "/api/settings/logo" : "",
      })
    );
  } catch (_) {
    /* ignore */
  }
}

function applyCachedBranding() {
  try {
    const brand = JSON.parse(localStorage.getItem("dashdrop-brand") || "null");
    if (!brand) return;
    document.querySelectorAll("[data-brand-name]").forEach((el) => {
      if (brand.app_name) el.textContent = brand.app_name;
    });
    if (brand.has_logo && brand.logo_url) {
      document.querySelectorAll("[data-brand-logo]").forEach((el) => {
        setLogoElement(el, brand.logo_url);
      });
    }
    if (brand.primary_color && HEX_COLOR_RE.test(brand.primary_color)) {
      applyAccentColor(brand.primary_color);
    }
  } catch (_) {
    /* ignore */
  }
}

async function loadBranding() {
  applyCachedBranding();
  try {
    const res = await fetch("/api/settings");
    if (!res.ok) return;
    const data = await res.json();
    applyBranding(data);
  } catch (_) {
    /* ignore */
  }
}

function getStoredThemePreference() {
  try {
    const stored = localStorage.getItem("dashdrop-theme");
    if (stored === "light" || stored === "dark" || stored === "system") return stored;
  } catch (_) {
    /* ignore */
  }
  return "system";
}

function resolveTheme(preference) {
  if (preference === "light" || preference === "dark") return preference;
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function getPreferredTheme() {
  return resolveTheme(getStoredThemePreference());
}

function updateThemeToggle(theme) {
  const btn = document.getElementById("theme-toggle");
  if (!btn) return;
  const dark = theme === "dark";
  btn.setAttribute("aria-pressed", dark ? "true" : "false");
  btn.setAttribute("aria-label", dark ? "Switch to light mode" : "Switch to dark mode");
  btn.title = dark ? "Switch to light mode" : "Switch to dark mode";
}

function applyTheme(preference, { persist = true } = {}) {
  const pref = preference === "light" || preference === "dark" || preference === "system" ? preference : "system";
  const next = resolveTheme(pref);
  document.documentElement.setAttribute("data-theme", next);
  if (persist) {
    try {
      localStorage.setItem("dashdrop-theme", pref);
    } catch (_) {
      /* ignore */
    }
  }
  updateThemeToggle(next);
  // Re-derive accent soft/glow for the active theme.
  const brandPrimary = document.documentElement.style.getPropertyValue("--brand-primary").trim();
  if (HEX_COLOR_RE.test(brandPrimary)) {
    applyAccentColor(brandPrimary);
  }
}

function initThemeToggle() {
  applyTheme(getStoredThemePreference());
  const btn = document.getElementById("theme-toggle");
  if (btn) {
    btn.addEventListener("click", () => {
      const current = document.documentElement.getAttribute("data-theme") === "dark" ? "dark" : "light";
      applyTheme(current === "dark" ? "light" : "dark");
      const radios = document.querySelectorAll('input[name="theme"]');
      radios.forEach((radio) => {
        radio.checked = radio.value === (current === "dark" ? "light" : "dark");
      });
    });
  }
  try {
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
      if (getStoredThemePreference() === "system") {
        applyTheme("system");
      }
    });
  } catch (_) {
    /* ignore */
  }
}

function normalizeHexInput(value) {
  let v = (value || "").trim();
  if (!v.startsWith("#")) v = "#" + v;
  if (/^#[0-9a-fA-F]{3}$/.test(v)) {
    v = "#" + v[1] + v[1] + v[2] + v[2] + v[3] + v[3];
  }
  return v.toLowerCase();
}

function initSettingsPage() {
  const form = document.getElementById("settings-form");
  if (!form) return;

  const nameInput = document.getElementById("setting-app-name");
  const colorPicker = document.getElementById("setting-color-picker");
  const colorHex = document.getElementById("setting-primary-color");
  const logoInput = document.getElementById("setting-logo");
  const removeLogoBtn = document.getElementById("btn-remove-logo");
  const logoPreview = document.querySelector("[data-logo-preview]");
  const errorEl = document.getElementById("settings-error");
  const statusEl = document.getElementById("settings-status");
  const saveBtn = document.getElementById("btn-save-settings");

  let currentSettings = {
    app_name: "Dashdrop",
    primary_color: DEFAULT_PRIMARY_LIGHT,
    has_logo: false,
    logo_url: "",
  };

  function showError(msg) {
    if (errorEl) {
      errorEl.textContent = msg || "";
      errorEl.style.display = msg ? "block" : "none";
    }
    if (statusEl && msg) {
      statusEl.textContent = "";
      statusEl.classList.remove("error");
    }
  }

  function showStatus(msg, isError) {
    if (!statusEl) return;
    statusEl.textContent = msg || "";
    statusEl.classList.toggle("error", !!isError);
  }

  function syncColorInputs(hex) {
    const normalized = normalizeHexInput(hex);
    if (!HEX_COLOR_RE.test(normalized)) return false;
    colorPicker.value = normalized;
    colorHex.value = normalized;
    applyAccentColor(normalized);
    return true;
  }

  function updateLogoPreview(settings) {
    setLogoElement(logoPreview, settings.has_logo && settings.logo_url ? settings.logo_url + "?t=" + Date.now() : "");
    if (removeLogoBtn) {
      removeLogoBtn.style.display = settings.has_logo ? "inline-flex" : "none";
    }
  }

  function fillForm(settings) {
    currentSettings = settings;
    nameInput.value = settings.app_name || "Dashdrop";
    const color =
      settings.primary_color && HEX_COLOR_RE.test(settings.primary_color)
        ? settings.primary_color.toLowerCase()
        : document.documentElement.getAttribute("data-theme") === "dark"
          ? DEFAULT_PRIMARY_DARK
          : DEFAULT_PRIMARY_LIGHT;
    syncColorInputs(color);
    updateLogoPreview(settings);

    const pref = getStoredThemePreference();
    document.querySelectorAll('input[name="theme"]').forEach((radio) => {
      radio.checked = radio.value === pref;
    });
  }

  colorPicker.addEventListener("input", () => {
    syncColorInputs(colorPicker.value);
  });

  colorHex.addEventListener("input", () => {
    const normalized = normalizeHexInput(colorHex.value);
    if (HEX_COLOR_RE.test(normalized)) {
      colorPicker.value = normalized;
      applyAccentColor(normalized);
      showError("");
    }
  });

  colorHex.addEventListener("blur", () => {
    const normalized = normalizeHexInput(colorHex.value);
    if (!syncColorInputs(normalized)) {
      showError("Primary color must be a hex value like #0f766e");
      colorHex.value = colorPicker.value;
    }
  });

  document.querySelectorAll('input[name="theme"]').forEach((radio) => {
    radio.addEventListener("change", () => {
      if (radio.checked) applyTheme(radio.value);
    });
  });

  logoInput.addEventListener("change", async () => {
    const file = logoInput.files && logoInput.files[0];
    logoInput.value = "";
    if (!file) return;
    if (file.size > 512 * 1024) {
      showError("Logo exceeds maximum size (512 KB)");
      return;
    }
    showError("");
    showStatus("Uploading logo...");
    const body = new FormData();
    body.append("logo", file);
    try {
      const res = await fetch("/api/settings/logo", { method: "POST", body });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        showError(data.error || "Failed to upload logo");
        showStatus("");
        return;
      }
      fillForm(data);
      applyBranding(data);
      showStatus("Logo updated");
    } catch (_) {
      showError("Failed to upload logo");
      showStatus("");
    }
  });

  removeLogoBtn.addEventListener("click", async () => {
    showError("");
    showStatus("Removing logo...");
    try {
      const res = await fetch("/api/settings/logo", { method: "DELETE" });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        showError(data.error || "Failed to remove logo");
        showStatus("");
        return;
      }
      fillForm(data);
      applyBranding(data);
      showStatus("Logo removed");
    } catch (_) {
      showError("Failed to remove logo");
      showStatus("");
    }
  });

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    showError("");
    const appName = nameInput.value.trim();
    const primaryColor = normalizeHexInput(colorHex.value);
    if (!appName || appName.length > 64) {
      showError("App name must be 1–64 characters");
      return;
    }
    if (!HEX_COLOR_RE.test(primaryColor)) {
      showError("Primary color must be a hex value like #0f766e");
      return;
    }
    saveBtn.disabled = true;
    showStatus("Saving...");
    try {
      const res = await fetch("/api/settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ app_name: appName, primary_color: primaryColor }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        showError(data.error || "Failed to save settings");
        showStatus("");
        return;
      }
      fillForm(data);
      applyBranding(data);
      showStatus("Settings saved");
    } catch (_) {
      showError("Failed to save settings");
      showStatus("");
    } finally {
      saveBtn.disabled = false;
    }
  });

  (async () => {
    try {
      const res = await fetch("/api/settings");
      if (!res.ok) throw new Error("load failed");
      const data = await res.json();
      fillForm(data);
      applyBranding(data);
    } catch (_) {
      fillForm(currentSettings);
      showError("Could not load settings");
    }
  })();
}

document.addEventListener("DOMContentLoaded", () => {
  initThemeToggle();
  loadBranding();
  initUploadPage();
  initLibraryPage();
  initSettingsPage();
});
