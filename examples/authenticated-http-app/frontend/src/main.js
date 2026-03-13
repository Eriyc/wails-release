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
const restartButton = document.getElementById("restart-app");

let currentState = null;

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
    currentState = state;
    const metadata = state.metadata || {};

    renderPairs(stateGrid, [
        ["App", metadata.appName],
        ["Source", metadata.sourceLabel],
        ["Proxy base URL", metadata.baseURL],
        ["Manifest URL", state.manifestURL],
        ["Current version", state.currentVersion],
        ["Current hash", state.currentHash],
        ["Channel", state.channel],
        ["Native compat", state.nativeCompat],
        ["Auth token configured", metadata.authConfigured === "true" ? "Yes" : "No"],
        ["Last checked", state.lastCheckedAt],
        ["Pending restart", state.pendingRestart ? "Yes" : "No"],
        ["Last error", state.lastError],
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

function renderResult(result, state = currentState) {
    const pendingRestart = Boolean(state?.pendingRestart);

    if (!result || !result.available || !result.update) {
        renderPairs(resultGrid, [
            ["Checked at", result?.checkedAt || new Date().toISOString()],
            ["Available", pendingRestart ? "Staged" : "No"],
            ["Pending restart", pendingRestart ? "Yes" : "No"],
            ["Message", result?.error || (pendingRestart ? "Update staged. Restart to launch it." : "No update cached.")],
        ]);
        return;
    }

    renderPairs(resultGrid, [
        ["Checked at", result.checkedAt],
        ["Available", "Yes"],
        ["Pending restart", pendingRestart ? "Yes" : "No"],
        ["Version", result.update.version],
        ["Channel", result.update.channel],
        ["Release notes", result.update.releaseNotes],
        ["Artifact URL", result.update.artifactURL],
        ["Artifact hash", result.update.artifactHash],
        ["Artifact size", formatBytes(result.update.artifactSize)],
        ["Delta URL", result.update.deltaURL],
        ["Delta hash", result.update.deltaHash],
        ["Delta size", formatBytes(result.update.deltaSize)],
        ["Delta from hash", result.update.deltaFromHash],
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
    const state = await UpdateService.GetState();
    renderState(state);
    renderResult({
        checkedAt: state.lastCheckedAt || new Date().toISOString(),
        available: Boolean(state.availableUpdate),
        update: state.availableUpdate,
        error: state.lastError,
    }, state);
}

function setBusy(isBusy) {
    refreshButton.disabled = isBusy;
    checkButton.disabled = isBusy;
    applyButton.disabled = isBusy;
    restartButton.disabled = isBusy;
}

refreshButton.addEventListener("click", async () => {
    await refreshState();
});

checkButton.addEventListener("click", async () => {
    setBusy(true);
    try {
        const result = await UpdateService.CheckNow();
        renderResult(result, currentState);
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
        const result = await UpdateService.ApplyPending();
        pushLog(result.error ? "error" : "info", result.error || result.message, result.startedAt);
        await refreshState();
    } finally {
        setBusy(false);
    }
});

restartButton.addEventListener("click", async () => {
    setBusy(true);
    try {
        const result = await UpdateService.Restart();
        pushLog(result.error ? "error" : "info", result.error || result.message, result.startedAt);
        if (!result.restarted) {
            await refreshState();
        }
    } finally {
        setBusy(false);
    }
});

Events.On("update:state", (event) => {
    renderState(event.data);
    renderResult({
        checkedAt: event.data.lastCheckedAt || new Date().toISOString(),
        available: Boolean(event.data.availableUpdate),
        update: event.data.availableUpdate,
        error: event.data.lastError,
    }, event.data);
});

Events.On("update:log", (event) => {
    pushLog(event.data.level, event.data.message, event.data.at);
});

Events.On("update:progress", (event) => {
    renderProgress(event.data);
});

async function bootstrap() {
    await refreshState();
    pushLog("info", "Ready. Start the Bun proxy, set the base URL and token, then check, stage, and restart explicitly.");
}

bootstrap().catch((error) => {
    console.error(error);
    pushLog("error", error instanceof Error ? error.message : String(error));
});
