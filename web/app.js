const page = document.body.dataset.page;

const $ = (selector) => document.querySelector(selector);

async function api(path, options = {}) {
  const response = await fetch(path, {
    credentials: "same-origin",
    cache: "no-store",
    ...options,
    headers: {
      ...(options.body instanceof FormData ? {} : { "Content-Type": "application/json" }),
      ...(options.headers || {}),
    },
  });
  const contentType = response.headers.get("Content-Type") || "";
  const payload = contentType.includes("application/json") ? await response.json() : null;
  if (!response.ok) {
    throw new Error((payload && payload.error) || `请求失败：${response.status}`);
  }
  return payload;
}

function formatBytes(bytes) {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function setText(selector, value) {
  const node = $(selector);
  if (node) node.textContent = value;
}

function showToast(message, isError = false) {
  const toast = $("#toast");
  if (!toast) return;
  toast.textContent = message;
  toast.classList.toggle("error", isError);
  toast.hidden = false;
  window.clearTimeout(showToast.timer);
  showToast.timer = window.setTimeout(() => {
    toast.hidden = true;
  }, 3200);
}

async function loadPublish() {
  const status = $("#publishStatus");
  const actions = $("#installActions");
  const qrPanel = $("#qrPanel");
  const qrImage = $("#qrImage");
  const installLink = $("#installLink");
  const hint = $("#publishHint");
  const releaseNotes = $("#releaseNotes");
  try {
    const state = await api("/api/publish");
    setText("#publishAppName", state.config.appName || "iOS App");
    if (!state.ready) {
      status.textContent = "未发布";
      hint.textContent = "当前还没有可安装的 IPA 和 plist";
      releaseNotes.hidden = true;
      actions.hidden = true;
      qrPanel.hidden = true;
      return;
    }
    status.textContent = "可安装";
    hint.textContent = "请使用 iPhone 点击安装，或扫描右侧二维码";
    installLink.href = state.installUrl;
    qrImage.src = `${state.qrUrl}?t=${Date.now()}`;
    setText("#installUrl", state.installUrl);
    const notes = (state.config.releaseNotes || "").trim();
    releaseNotes.textContent = notes;
    releaseNotes.hidden = notes === "";
    actions.hidden = false;
    qrPanel.hidden = false;
  } catch (error) {
    status.textContent = "加载失败";
    hint.textContent = error.message;
    releaseNotes.hidden = true;
    actions.hidden = true;
    qrPanel.hidden = true;
  }
}

async function loadAdmin() {
  const state = await api("/api/state");
  const form = $("#configForm");
  form.elements.appName.value = state.config.appName || "";
  form.elements.ipaUrl.value = state.config.ipaUrl || "";
  form.elements.plistUrl.value = state.config.plistUrl || "";
  form.elements.releaseNotes.value = state.config.releaseNotes || "";

  const plistForm = $("#plistForm");
  if (!plistForm.elements.title.value) {
    plistForm.elements.title.value = state.config.appName || "";
  }

  setText("#adminAppName", state.config.appName || "iOS App");
  setText("#ipaState", state.hasIpa ? formatBytes(state.ipaSize) : "未上传");
  setText("#plistState", state.hasPlist ? formatBytes(state.plistSize) : "未生成");
  setText("#uploadLimit", formatBytes(state.maxUploadBytes));
  setText("#configSavedAt", state.config.updatedAt ? new Date(state.config.updatedAt).toLocaleString() : "");

  $("#adminManifestLink").href = state.config.plistUrl || "/manifest.plist";
  $("#adminIPALink").href = state.config.ipaUrl || "/files/app.ipa";
  $("#adminQRLink").href = state.qrUrl || "/qr.png";

  const downloadPlistButton = $("#downloadPlistButton");
  downloadPlistButton.hidden = !state.hasPlist;
  downloadPlistButton.href = `/manifest.plist?download=1&t=${Date.now()}`;
}

function bindAdmin() {
  $("#configForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const button = event.submitter;
    button.disabled = true;
    try {
      const form = event.currentTarget;
      await api("/api/config", {
        method: "POST",
        body: JSON.stringify({
          appName: form.elements.appName.value,
          releaseNotes: form.elements.releaseNotes.value,
          ipaUrl: form.elements.ipaUrl.value,
          plistUrl: form.elements.plistUrl.value,
        }),
      });
      await loadAdmin();
      showToast("配置已保存");
    } catch (error) {
      showToast(error.message, true);
    } finally {
      button.disabled = false;
    }
  });

  $("#uploadForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const button = event.submitter;
    button.disabled = true;
    try {
      const form = event.currentTarget;
      const data = new FormData(form);
      await api("/api/upload", { method: "POST", body: data });
      form.reset();
      await loadAdmin();
      showToast("IPA 已上传");
    } catch (error) {
      showToast(error.message, true);
    } finally {
      button.disabled = false;
    }
  });

  $("#plistForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const button = event.submitter;
    button.disabled = true;
    try {
      const form = event.currentTarget;
      await api("/api/plist/generate", {
        method: "POST",
        body: JSON.stringify({
          bundleIdentifier: form.elements.bundleIdentifier.value,
          bundleVersion: form.elements.bundleVersion.value,
          title: form.elements.title.value,
        }),
      });
      await loadAdmin();
      showToast("plist 已生成");
    } catch (error) {
      showToast(error.message, true);
    } finally {
      button.disabled = false;
    }
  });
}

if (page === "publish") {
  loadPublish();
}

if (page === "internal") {
  bindAdmin();
  loadAdmin().catch((error) => showToast(error.message, true));
}
