import { Events } from "@wailsio/runtime";
import { UpdateService } from "../bindings/github.com/Eriyc/wailsrel/examples/authenticated-http-app/index.js";

const stateGrid = document.getElementById("state-grid");
const stateNotes = document.getElementById("state-notes");
const resultGrid = document.getElementById("result-grid");
const progressBar = document.getElementById("progress-bar");
const progressText = document.getElementById("progress-text");
const log = document.getElementById("log");
const refreshButton = document.getElementById("refresh-state");
const checkButton = document.getElementById("check-update");
const applyButton = document.getElementById("apply-update");

function renderPairs(container, pairs) {
    container.innerHTML = "";
    for (const [label, value] of pairs) {
        const row = document.createElement("div");
        row.className = "kv-row";

        const key = document.createElement("span");
        key.className = "kv-key";
        key.textContent = label;

        const val = document.createElement("span");
        val.className = "kv-value";
        val.textContent = value || "—";

        row.append(key, val);
        container.append(row);
    }
}

function pushLog(level, message, at = new Date().toISOString()) {
    const item = document.createElement("div");
    item.className = `log-line ${level}`;
    item.textContent = `${new Date(at).toLocaleTimeString()}  ${level.toUpperCase()}  ${message}`;
    log.prepend(item);
}

function formatBytes(value) {
    if (!value) {
        return "0 B";
    }
    const units = ["B", "KB", "MB", "GB"];
    let size = value;
    let unit = units[0];
    for (let index = 0; index < units.length - 1 && size >= 1024; index += 1) {
        size /= 1024;
        unit = units[index + 1];
    }
    return `${size.toFixed(size >= 10 ? 0 : 1)} ${unit}`;
}

function renderState(state) {
    renderPairs(stateGrid, [
        ["App", state.appName],
        ["Source", state.sourceLabel],
        ["Proxy base URL", state.baseURL],
        ["Manifest URL", state.manifestURL],
        ["Current version", state.currentVersion],
        ["Current hash", state.currentHash],
        ["Channel", state.channel],
        ["Native compat", state.nativeCompat],
        ["Auth token configured", state.authConfigured ? "Yes" : "No"],
        ["Target path", state.targetPath],
        ["Temp dir", state.tempDir],
    ]);

    stateNotes.innerHTML = "";
    for (const note of state.notes || []) {
        const line = document.createElement("p");
        line.textContent = note;
        stateNotes.append(line);
    }
}

function renderResult(result) {
    if (!result || !result.available || !result.update) {
        renderPairs(resultGrid, [
            ["Checked at", result?.checkedAt || new Date().toISOString()],
            ["Available", "No"],
            ["Message", result?.error || "No update cached."],
        ]);
        return;
    }

    renderPairs(resultGrid, [
        ["Checked at", result.checkedAt],
        ["Available", "Yes"],
        ["Version", result.update.version],
        ["Channel", result.update.channel],
        ["Artifact URL", result.update.artifactURL],
        ["Artifact hash", result.update.artifactHash],
        ["Artifact size", formatBytes(result.update.artifactSize)],
        ["Delta URL", result.update.deltaURL],
        ["Delta hash", result.update.deltaHash],
        ["Delta size", formatBytes(result.update.deltaSize)],
        ["Mandatory", result.update.mandatory ? "Yes" : "No"],
    ]);
}

function renderProgress(progress) {
    const total = Number(progress.total || 0);
    const downloaded = Number(progress.downloaded || 0);
    const percent = total > 0 ? Math.min(100, (downloaded / total) * 100) : 6;
    progressBar.style.width = `${percent}%`;
    progressText.textContent = total > 0
        ? `${formatBytes(downloaded)} of ${formatBytes(total)}`
        : `${formatBytes(downloaded)} downloaded`;
}

async function refreshState() {
    renderState(await UpdateService.GetState());
}

function setBusy(isBusy) {
    refreshButton.disabled = isBusy;
    checkButton.disabled = isBusy;
    applyButton.disabled = isBusy;
}

refreshButton.addEventListener("click", async () => {
    await refreshState();
});

checkButton.addEventListener("click", async () => {
    setBusy(true);
    try {
        const result = await UpdateService.CheckNow();
        renderResult(result);
        await refreshState();
        if (result.error) {
            pushLog("error", result.error, result.checkedAt);
        }
    } finally {
        setBusy(false);
    }
});

applyButton.addEventListener("click", async () => {
    setBusy(true);
    try {
        const result = await UpdateService.ApplyLastUpdate();
        pushLog(result.error ? "error" : "info", result.error || result.message, result.startedAt);
        await refreshState();
    } finally {
        setBusy(false);
    }
});

Events.On("update:log", (event) => {
    pushLog(event.data.level, event.data.message, event.data.at);
});

Events.On("update:progress", (event) => {
    renderProgress(event.data);
});

Events.On("update:available", (event) => {
    renderResult({
        checkedAt: new Date().toISOString(),
        available: true,
        update: event.data,
    });
});

async function bootstrap() {
    await refreshState();
    renderResult({ checkedAt: new Date().toISOString(), available: false });
    pushLog("info", "Ready. Start the Bun proxy, set the base URL and token, then click check.");
}

bootstrap().catch((error) => {
    console.error(error);
    pushLog("error", error instanceof Error ? error.message : String(error));
});
