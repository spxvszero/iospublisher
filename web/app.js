const page = document.body.dataset.page;

const $ = (selector) => document.querySelector(selector);

const adminState = {
  currentTag: localStorage.getItem("iospublisher.currentTag") || "default",
  tags: [],
};

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

function formatDate(value) {
  if (!value || String(value).startsWith("0001-")) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleString();
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

function tagQuery(tag = adminState.currentTag) {
  return tag && tag !== "default" ? `?tag=${encodeURIComponent(tag)}` : "";
}

function urlWithParams(path, params) {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== "") {
      query.set(key, value);
    }
  });
  const text = query.toString();
  return text ? `${path}?${text}` : path;
}

function cacheBust(url) {
  return `${url}${url.includes("?") ? "&" : "?"}t=${Date.now()}`;
}

function ipaFilePath(state) {
  return `/files/${encodeURIComponent(state.ipaFilename || "app.ipa")}`;
}

function createNode(tag, attrs = {}, children = []) {
  const node = document.createElement(tag);
  Object.entries(attrs).forEach(([key, value]) => {
    if (key === "class") node.className = value;
    else if (key === "text") node.textContent = value;
    else if (key === "html") node.innerHTML = value;
    else if (key === "hidden") node.hidden = Boolean(value);
    else if (value !== false && value !== null && value !== undefined) node.setAttribute(key, value);
  });
  for (const child of Array.isArray(children) ? children : [children]) {
    if (child === null || child === undefined) continue;
    node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return node;
}

async function loadPublish() {
  const list = $("#publishList");
  const emptyState = $("#publishEmpty");
  list.replaceChildren();

  try {
    const payload = await api("/api/publish");
    const tags = Array.isArray(payload.tags) && payload.tags.length ? payload.tags : [payload];
    if (tags.length === 1 && !tags[0].ready) {
      list.hidden = true;
      emptyState.hidden = false;
      return;
    }

    emptyState.hidden = true;
    list.hidden = false;
    if (tags.length === 1) {
      list.className = "publish-single";
      list.append(renderPackage(tags[0]));
      return;
    }

    list.className = "publish-list";
    tags.forEach((state) => {
      const details = createNode("details", { class: "publish-entry" });
      details.open = state.isDefault;
      const title = `${state.config.appName || "iOS App"} ${state.tag}`;
      const publishedAt = formatDate(state.config.publishedAt) || "未发布";
      details.append(
        createNode("summary", {}, [
          createNode("strong", { text: title }),
          createNode("span", { class: "muted", text: publishedAt }),
        ]),
        renderPackage(state),
      );
      list.append(details);
    });
  } catch (error) {
    list.hidden = false;
    emptyState.hidden = true;
    list.className = "publish-single";
    list.append(
      createNode("section", { class: "publish-main" }, [
        createNode("div", { class: "status-line", text: "加载失败" }),
        createNode("h1", { text: "iOS App" }),
        createNode("p", { class: "muted", text: error.message }),
      ]),
    );
  }
}

function renderPackage(state) {
  const layout = createNode("div", { class: "package-layout" });
  const main = createNode("section", { class: "publish-main" });
  const publishedAt = formatDate(state.config.publishedAt);
  main.append(
    createNode("div", { class: "status-line", text: state.ready ? "可安装" : "未配置" }),
    createNode("h1", { text: state.config.appName || "iOS App" }),
    createNode("p", { class: "muted", text: `发布时间：${publishedAt || "未发布"}` }),
  );

  if (state.ready) {
    const actions = createNode("div", { class: "install-actions" }, [
      createNode("a", { class: "primary-button", href: state.installUrl, text: "安装" }),
    ]);
    main.append(actions, createNode("p", { class: "muted", text: "请使用 iPhone 点击安装，或扫描二维码" }));
  } else {
    main.append(createNode("p", { class: "muted", text: "当前标签尚未完成 IPA 上传和 plist 生成" }));
  }

  const notes = (state.config.releaseNotes || "").trim();
  if (notes) {
    main.append(createNode("div", { class: "release-notes", text: notes }));
  }
  layout.append(main);

  if (state.ready) {
    layout.append(
      createNode("section", { class: "qr-panel" }, [
        createNode("img", { src: cacheBust(state.qrUrl), alt: "安装二维码" }),
        createNode("div", { class: "url-box", text: state.installUrl }),
      ]),
    );
  }

  if (canSearchUUID(state)) {
    layout.append(renderUUIDSearch(state));
  }
  return layout;
}

function canSearchUUID(state) {
  const analysis = state.analysis || {};
  return analysis.packageType === "development" && Array.isArray(analysis.deviceUUIDs) && analysis.deviceUUIDs.length > 0;
}

function renderUUIDSearch(state) {
  const panel = createNode("section", { class: "uuid-panel" });
  const form = createNode("form", { class: "uuid-form" });
  const input = createNode("input", {
    name: "q",
    minlength: "4",
    autocomplete: "off",
    placeholder: "输入至少 4 位 UUID 片段",
  });
  const result = createNode("div", { class: "uuid-result muted" });
  form.append(input, createNode("button", { class: "secondary-button", type: "submit", text: "查询 UUID" }));
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    result.textContent = "查询中...";
    try {
      const payload = await api(urlWithParams("/api/uuid/search", {
        tag: state.isDefault ? "" : state.tag,
        q: input.value,
      }));
      if (!payload.matches.length) {
        result.textContent = "未找到匹配 UUID";
        return;
      }
      result.replaceChildren(
        createNode("div", { text: `找到 ${payload.matches.length} 条匹配` }),
        createNode("div", { class: "uuid-list", text: payload.matches.join("\n") }),
      );
    } catch (error) {
      result.textContent = error.message;
    }
  });
  panel.append(
    createNode("div", { class: "section-head" }, [createNode("h2", { text: "UUID 查询" })]),
    form,
    result,
  );
  return panel;
}

async function loadAdmin() {
  const payload = await api("/api/tags");
  adminState.tags = payload.tags || [];
  if (!adminState.tags.some((item) => item.tag === adminState.currentTag)) {
    adminState.currentTag = payload.activeTag || "default";
  }
  const state = adminState.tags.find((item) => item.tag === adminState.currentTag) || adminState.tags[0];
  if (!state) return;
  adminState.currentTag = state.tag;
  localStorage.setItem("iospublisher.currentTag", adminState.currentTag);

  renderTagTabs();
  fillAdmin(state);
}

function renderTagTabs() {
  const tabs = $("#tagTabs");
  tabs.replaceChildren();
  adminState.tags.forEach((state) => {
    const button = createNode("button", {
      type: "button",
      class: `tag-tab${state.tag === adminState.currentTag ? " active" : ""}`,
    }, [
      createNode("span", { text: state.tag }),
      createNode("small", { text: state.ready ? "已发布" : "未发布" }),
    ]);
    button.addEventListener("click", () => {
      adminState.currentTag = state.tag;
      localStorage.setItem("iospublisher.currentTag", adminState.currentTag);
      loadAdmin().catch((error) => showToast(error.message, true));
    });
    tabs.append(button);
  });
}

function fillAdmin(state) {
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
  setText("#activeTagName", state.tag);
  setText("#ipaState", ipaStateText(state));
  setText("#plistState", state.hasPlist ? `${state.plistFilename} · ${formatBytes(state.plistSize)}` : "未生成");
  setText("#publishedAtState", formatDate(state.config.publishedAt) || "未发布");
  setText("#uploadLimit", formatBytes(state.maxUploadBytes));
  setText("#configSavedAt", formatDate(state.config.updatedAt));
  renderCurrentIPA(state);

  $("#adminPublishLink").href = "/publish";
  $("#adminManifestLink").href = state.config.plistUrl || "/manifest.plist";
  $("#adminManifestLink").textContent = state.plistFilename || "manifest.plist";
  $("#adminIPALink").href = state.config.ipaUrl || ipaFilePath(state);
  $("#adminIPALink").textContent = state.ipaFilename || "app.ipa";
  $("#adminQRLink").href = state.qrUrl || "/qr.png";

  const downloadPlistButton = $("#downloadPlistButton");
  downloadPlistButton.hidden = !state.hasPlist;
  downloadPlistButton.href = state.isDefault
    ? `/manifest.plist?download=1&t=${Date.now()}`
    : `/${state.plistFilename}?download=1&t=${Date.now()}`;
  downloadPlistButton.download = state.plistFilename || "manifest.plist";

  const deleteButton = $("#deleteTagButton");
  deleteButton.disabled = state.isDefault;
  deleteButton.hidden = state.isDefault;
  renderAnalysis(state.analysis || {});
}

function renderCurrentIPA(state) {
  const panel = $("#currentIPAPanel");
  const link = $("#currentIPALink");
  const meta = $("#currentIPAMeta");
  const deleteButton = $("#deleteIPAButton");
  if (!panel || !link || !meta || !deleteButton) return;

  panel.hidden = !state.hasIpa;
  deleteButton.disabled = !state.hasIpa;
  if (!state.hasIpa) {
    link.removeAttribute("href");
    link.removeAttribute("download");
    link.textContent = "";
    meta.textContent = "";
    return;
  }

  const filename = state.ipaFilename || "app.ipa";
  const createdAt = formatDate(state.ipaCreatedAt) || formatDate(state.config.publishedAt) || "-";
  link.href = `${ipaFilePath(state)}?t=${Date.now()}`;
  link.download = filename;
  link.textContent = filename;
  meta.textContent = `创建日期：${createdAt} · ${formatBytes(state.ipaSize)}`;
}

function ipaStateText(state) {
  if (state.hasIpa) {
    return `${state.ipaFilename} · ${formatBytes(state.ipaSize)}`;
  }
  if (state.remoteIpa) {
    return "远端链接";
  }
  return "未上传";
}

function renderAnalysis(analysis) {
  const statusLabels = {
    pending: "等待上传",
    success: "已分析",
    failed: "分析失败",
  };
  const packageLabels = {
    development: "开发包",
    "ad-hoc": "Ad Hoc",
    enterprise: "企业包",
    "app-store": "App Store",
    unknown: "未知",
  };
  setText("#analysisStatus", statusLabels[analysis.status] || "等待上传");
  setText("#packageType", packageLabels[analysis.packageType] || "未知");
  setText("#deviceUUIDCount", Array.isArray(analysis.deviceUUIDs) ? `${analysis.deviceUUIDs.length} 个` : "0 个");
  setText("#certificateExpiresAt", formatDate(analysis.certificateExpiresAt) || "-");
  setText("#profileExpiresAt", formatDate(analysis.profileExpiresAt) || "-");
  const error = $("#analysisError");
  error.textContent = analysis.error || "";
  error.hidden = !analysis.error;
}

function bindAdmin() {
  $("#tagForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const button = event.submitter;
    button.disabled = true;
    try {
      const state = await api("/api/tags", {
        method: "POST",
        body: JSON.stringify({ name: form.elements.name.value }),
      });
      adminState.currentTag = state.tag;
      form.reset();
      await loadAdmin();
      showToast("标签已新增");
    } catch (error) {
      showToast(error.message, true);
    } finally {
      button.disabled = false;
    }
  });

  $("#deleteTagButton").addEventListener("click", async (event) => {
    const button = event.currentTarget;
    if (!adminState.currentTag || adminState.currentTag === "default") return;
    if (!window.confirm(`删除标签 ${adminState.currentTag}？对应 IPA、plist 和分析数据也会删除。`)) return;
    button.disabled = true;
    try {
      await api(`/api/tags${tagQuery()}`, { method: "DELETE" });
      adminState.currentTag = "default";
      await loadAdmin();
      showToast("标签已删除");
    } catch (error) {
      showToast(error.message, true);
    } finally {
      button.disabled = false;
    }
  });

  $("#deleteIPAButton").addEventListener("click", async (event) => {
    const button = event.currentTarget;
    if (!window.confirm(`删除当前标签 ${adminState.currentTag} 的 IPA 包？`)) return;
    button.disabled = true;
    try {
      await api(`/api/upload${tagQuery()}`, { method: "DELETE" });
      await loadAdmin();
      showToast("IPA 已删除");
    } catch (error) {
      showToast(error.message, true);
    } finally {
      button.disabled = false;
    }
  });

  $("#configForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const button = event.submitter;
    button.disabled = true;
    try {
      const form = event.currentTarget;
      await api(`/api/config${tagQuery()}`, {
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
      await api(`/api/upload${tagQuery()}`, { method: "POST", body: data });
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
      await api(`/api/plist/generate${tagQuery()}`, {
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
