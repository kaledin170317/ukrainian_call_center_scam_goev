function randomID() {
    if (globalThis.crypto && typeof globalThis.crypto.randomUUID === "function") {
        return globalThis.crypto.randomUUID();
    }
    return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

const API_BASE = "";

function qs(id) {
    return document.getElementById(id);
}

function setStatus(el, kind, text) {
    el.classList.remove("ok", "err");
    if (kind) el.classList.add(kind);
    el.textContent = text || "";
}

function setProgressDeterminate(el, value) {
    el.removeAttribute("indeterminate");
    el.value = Math.max(0, Math.min(100, Number(value || 0)));
}

function setProgressIndeterminate(el) {
    el.removeAttribute("value");
    el.setAttribute("indeterminate", "true");
}

function resetProgress(el, textEl, text = "") {
    setProgressDeterminate(el, 0);
    textEl.textContent = text;
}

function escapeHtml(value) {
    return String(value ?? "")
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#039;");
}

function kopToRub(kop) {
    return (Number(kop || 0) / 100).toFixed(2);
}

function wireDropzone(drop, input, onFile) {
    drop.addEventListener("dragover", (e) => {
        e.preventDefault();
        drop.classList.add("dragover");
    });

    drop.addEventListener("dragleave", () => drop.classList.remove("dragover"));

    drop.addEventListener("drop", (e) => {
        e.preventDefault();
        drop.classList.remove("dragover");
        const file = e.dataTransfer.files && e.dataTransfer.files[0];
        if (file) onFile(file);
    });

    drop.addEventListener("click", (e) => {
        if (e.target && e.target.closest("button")) return;
        input.click();
    });

    input.addEventListener("change", () => {
        const file = input.files && input.files[0];
        if (file) onFile(file);
        input.value = "";
    });
}

function uploadMultipart({ file, url, progressEl, progressTextEl, statusEl, onSuccess }) {
    const xhr = new XMLHttpRequest();
    const form = new FormData();
    form.append("file", file, file.name);

    setProgressDeterminate(progressEl, 0);
    progressTextEl.textContent = "0%";
    setStatus(statusEl, null, "Загрузка...");

    xhr.open("POST", url, true);

    xhr.upload.onprogress = (e) => {
        if (!e.lengthComputable) return;
        const pct = Math.round((e.loaded / e.total) * 100);
        setProgressDeterminate(progressEl, pct);
        progressTextEl.textContent = `${pct}% (${Math.round(e.loaded / 1024)} KB / ${Math.round(e.total / 1024)} KB)`;
    };

    xhr.onload = () => {
        if (xhr.status < 200 || xhr.status >= 300) {
            setStatus(statusEl, "err", `Ошибка (HTTP ${xhr.status}): ${xhr.responseText || "unknown"}`);
            return;
        }

        setProgressDeterminate(progressEl, 100);
        progressTextEl.textContent = "100%";

        let json = null;
        try {
            json = xhr.responseText ? JSON.parse(xhr.responseText) : null;
        } catch (_) {
            setStatus(statusEl, "err", "Не удалось распарсить ответ сервера");
            return;
        }

        onSuccess(json);
    };

    xhr.onerror = () => {
        setStatus(statusEl, "err", "Ошибка сети при загрузке");
    };

    xhr.send(form);
}

function setupReferenceUpload(cfg) {
    const input = qs(cfg.inputId);
    const drop = qs(cfg.dropId);
    const browse = qs(cfg.browseId);
    const fileName = qs(cfg.fileNameId);
    const progress = qs(cfg.progressId);
    const progressText = qs(cfg.progressTextId);
    const status = qs(cfg.statusId);

    browse.addEventListener("click", () => input.click());

    wireDropzone(drop, input, (file) => {
        fileName.textContent = `${file.name} (${Math.round(file.size / 1024)} KB)`;
        uploadMultipart({
            file,
            url: cfg.url,
            progressEl: progress,
            progressTextEl: progressText,
            statusEl: status,
            onSuccess: () => setStatus(status, "ok", "Файл загружен"),
        });
    });
}

function setupPreparedCDR() {
    const input = qs("cdrFile");
    const drop = qs("cdrDrop");
    const browse = qs("cdrBrowse");
    const fileName = qs("cdrFileName");
    const preparedMeta = qs("cdrPreparedMeta");
    const uploadProgress = qs("cdrUploadProgress");
    const uploadProgressText = qs("cdrUploadProgressText");
    const uploadStatus = qs("cdrUploadStatus");
    const startBtn = qs("cdrStartBtn");
    const collectCalls = qs("collectCalls");
    const processingWrap = qs("cdrProcessingWrap");
    const processingProgress = qs("cdrProcessingProgress");
    const processingText = qs("cdrProcessingText");

    let preparedID = "";
    let pollTimer = null;
    let pollingActive = false;

    function stopPolling() {
        pollingActive = false;
        if (pollTimer) {
            clearTimeout(pollTimer);
            pollTimer = null;
        }
    }

    function startPolling(progressID, statusEl) {
        stopPolling();
        pollingActive = true;

        const poll = async () => {
            try {
                const resp = await fetch(`${API_BASE}/api/v1/cdr/progress/${encodeURIComponent(progressID)}`, { cache: "no-store" });
                if (resp.ok) {
                    const p = await resp.json();
                    if (p.progress_pct === null || Number(p.total_bytes || 0) <= 0) {
                        setProgressIndeterminate(processingProgress);
                        processingText.textContent = `Расчет... ${Math.round((p.read_bytes || 0) / 1024)} KB processed`;
                    } else {
                        const pct = Math.max(0, Math.min(100, Number(p.progress_pct || 0)));
                        setProgressDeterminate(processingProgress, pct);
                        processingText.textContent = `Расчет... ${pct}% (${Math.round((p.read_bytes || 0) / 1024)} KB / ${Math.round((p.total_bytes || 0) / 1024)} KB)`;
                    }

                    if (p.status === "error" && p.error) {
                        setStatus(statusEl, "err", `Ошибка расчета: ${p.error}`);
                    }
                }
            } catch (_) {
                // ignore polling glitches
            } finally {
                if (pollingActive) {
                    pollTimer = setTimeout(poll, 250);
                }
            }
        };

        pollTimer = setTimeout(poll, 250);
    }

    function resetPreparedState() {
        preparedID = "";
        startBtn.disabled = true;
        preparedMeta.textContent = "Файл еще не подготовлен";
        stopPolling();
        processingWrap.style.display = "none";
        resetProgress(processingProgress, processingText);
    }

    async function startCalculation() {
        if (!preparedID) return;

        const progressID = randomID();
        const started = performance.now();

        startBtn.disabled = true;
        processingWrap.style.display = "block";
        setProgressDeterminate(processingProgress, 0);
        processingText.textContent = "Расчет запущен...";
        setStatus(uploadStatus, null, "Идет расчет...");
        startPolling(progressID, uploadStatus);

        try {
            const resp = await fetch(`${API_BASE}/api/v1/cdr/start`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                    prepared_id: preparedID,
                    collect_calls: collectCalls.checked,
                    progress_id: progressID,
                }),
            });

            stopPolling();
            const payload = await resp.json().catch(() => null);
            if (!resp.ok) {
                const msg = payload?.error?.message || `HTTP ${resp.status}`;
                setStatus(uploadStatus, "err", `Ошибка расчета: ${msg}`);
                return;
            }

            const clientElapsed = Math.round((performance.now() - started) * 10) / 10;
            const calcMS = Number(payload?.calculation_ms || 0);
            setProgressDeterminate(processingProgress, 100);
            processingText.textContent = `Расчет завершен: ${calcMS.toFixed(1)} ms`;
            setStatus(uploadStatus, "ok", `Расчет завершен за ${calcMS.toFixed(1)} ms (клиент получил ответ за ${clientElapsed} ms)`);
            renderReport(payload);
        } catch (_) {
            stopPolling();
            setStatus(uploadStatus, "err", "Ошибка сети при запуске расчета");
        } finally {
            startBtn.disabled = !preparedID;
        }
    }

    browse.addEventListener("click", () => input.click());
    startBtn.addEventListener("click", startCalculation);

    wireDropzone(drop, input, (file) => {
        resetPreparedState();
        fileName.textContent = `${file.name} (${Math.round(file.size / 1024)} KB)`;
        setStatus(uploadStatus, null, "Загрузка и нормализация...");

        uploadMultipart({
            file,
            url: `${API_BASE}/api/v1/cdr/prepare`,
            progressEl: uploadProgress,
            progressTextEl: uploadProgressText,
            statusEl: uploadStatus,
            onSuccess: (json) => {
                preparedID = json?.prepared_id || "";
                if (!preparedID) {
                    setStatus(uploadStatus, "err", "Сервер не вернул prepared_id");
                    return;
                }

                preparedMeta.textContent = `Подготовлено: ${json.rows_count || 0} строк, ${Math.round(Number(json.normalized_bytes || 0) / 1024)} KB`;
                setStatus(uploadStatus, "ok", "Файл загружен и нормализован. Можно запускать расчет.");
                startBtn.disabled = false;
            },
        });
    });
}

function renderReport(report) {
    const meta = qs("resultMeta");
    const totalsWrap = qs("totalsWrap");
    const totalsTBody = qs("totalsTable").querySelector("tbody");
    const callsDetails = qs("callsDetails");
    const callsTBody = qs("callsTable").querySelector("tbody");

    if (!report || report.status !== "ok") {
        meta.textContent = "Пока пусто";
        totalsWrap.style.display = "none";
        callsDetails.style.display = "none";
        return;
    }

    const totals = Array.isArray(report.totals) ? report.totals : [];
    const calls = Array.isArray(report.calls) ? report.calls : [];
    const calcMS = Number(report.calculation_ms || 0);

    meta.textContent = `status=${report.status}, calculation_ms=${calcMS.toFixed(1)}, totals=${totals.length}, calls=${calls.length}`;

    totalsTBody.innerHTML = totals.map((t) => {
        const kop = Number(t.total_cost_kop || 0);
        return `
      <tr>
        <td>${escapeHtml(t.phone_number)}</td>
        <td>${escapeHtml(t.client_name || "")}</td>
        <td>${escapeHtml(t.calls_count)}</td>
        <td>${escapeHtml(kop)}</td>
        <td>${escapeHtml(kopToRub(kop))}</td>
      </tr>
    `;
    }).join("");

    totalsWrap.style.display = "block";

    if (calls.length > 0) {
        callsTBody.innerHTML = calls.map((c) => {
            const kop = Number(c.cost_kop || 0);
            const tariff = c.tariff
                ? `${escapeHtml(c.tariff.prefix)} → ${escapeHtml(c.tariff.destination)} (p=${escapeHtml(c.tariff.priority)})`
                : "";

            return `
        <tr>
          <td>${escapeHtml(c.start_time)}</td>
          <td>${escapeHtml(c.end_time)}</td>
          <td>${escapeHtml(c.calling_party)}</td>
          <td>${escapeHtml(c.called_party)}</td>
          <td>${escapeHtml(c.call_direction)}</td>
          <td>${escapeHtml(c.disposition)}</td>
          <td>${escapeHtml(c.duration)}</td>
          <td>${escapeHtml(c.billable_sec)}</td>
          <td>${escapeHtml(kop)}</td>
          <td>${escapeHtml(kopToRub(kop))}</td>
          <td>${tariff}</td>
        </tr>
      `;
        }).join("");

        callsDetails.style.display = "block";
    } else {
        callsDetails.style.display = "none";
    }
}

setupReferenceUpload({
    inputId: "tariffsFile",
    dropId: "tariffsDrop",
    browseId: "tariffsBrowse",
    fileNameId: "tariffsFileName",
    progressId: "tariffsProgress",
    progressTextId: "tariffsProgressText",
    statusId: "tariffsStatus",
    url: `${API_BASE}/api/v1/tariffs`,
});

setupReferenceUpload({
    inputId: "subsFile",
    dropId: "subsDrop",
    browseId: "subsBrowse",
    fileNameId: "subsFileName",
    progressId: "subsProgress",
    progressTextId: "subsProgressText",
    statusId: "subsStatus",
    url: `${API_BASE}/api/v1/subscribers`,
});

setupPreparedCDR();
renderReport(null);
