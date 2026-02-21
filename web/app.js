// Если бэкенд на другом хосте/порту — выставь, например: "http://localhost:8080"
const API_BASE = "";

// --- helpers ---
function qs(id) { return document.getElementById(id); }

function setStatus(el, kind, text) {
    el.classList.remove("ok", "err");
    el.classList.add(kind);
    el.textContent = text;
}

function resetProgress(progressEl, progressTextEl) {
    progressEl.value = 0;
    progressTextEl.textContent = "";
}

function kopToRub(kop) {
    // kop — int64, но в JS будет number
    const v = Number(kop || 0);
    return (v / 100).toFixed(2);
}

function escapeHtml(s) {
    return String(s ?? "")
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#039;");
}

// --- Upload core ---
function setupAutoUpload(opts) {
    const input = qs(opts.inputId);
    const drop = qs(opts.dropId);
    const browse = qs(opts.browseId);
    const fileName = qs(opts.fileNameId);
    const progress = qs(opts.progressId);
    const progressText = qs(opts.progressTextId);
    const status = qs(opts.statusId);

    function uploadFile(file) {
        if (!file) return;

        fileName.textContent = file.name;
        resetProgress(progress, progressText);
        setStatus(status, "ok", "Uploading...");

        const url = opts.urlBuilder();
        const form = new FormData();
        form.append("file", file);

        const started = performance.now();

        const xhr = new XMLHttpRequest();
        xhr.open("POST", url, true);

        xhr.upload.onprogress = (e) => {
            if (!e.lengthComputable) return;
            const pct = Math.round((e.loaded / e.total) * 100);
            progress.value = pct;
            progressText.textContent = `${pct}% (${Math.round(e.loaded/1024)} KB / ${Math.round(e.total/1024)} KB)`;
        };

        xhr.onload = () => {
            const elapsed = Math.round((performance.now() - started) * 10) / 10;

            if (xhr.status >= 200 && xhr.status < 300) {
                progress.value = 100;
                setStatus(status, "ok", `OK (HTTP ${xhr.status}, elapsed ~${elapsed} ms)`);

                let json = null;
                try { json = xhr.responseText ? JSON.parse(xhr.responseText) : null; } catch (_) {}

                if (opts.onSuccess) opts.onSuccess(json);
            } else {
                setStatus(status, "err", `Ошибка (HTTP ${xhr.status}): ${xhr.responseText || "unknown"}`);
            }
        };

        xhr.onerror = () => setStatus(status, "err", "Ошибка сети при загрузке");
        xhr.send(form);
    }

    // browse -> open file picker
    browse.addEventListener("click", () => input.click());

    // file picked -> auto upload
    input.addEventListener("change", () => {
        const file = input.files && input.files[0];
        uploadFile(file);
        // чтобы повторный выбор того же файла снова триггерил change
        input.value = "";
    });

    // drag&drop
    drop.addEventListener("dragover", (e) => {
        e.preventDefault();
        drop.classList.add("dragover");
    });
    drop.addEventListener("dragleave", () => drop.classList.remove("dragover"));
    drop.addEventListener("drop", (e) => {
        e.preventDefault();
        drop.classList.remove("dragover");
        const file = e.dataTransfer.files && e.dataTransfer.files[0];
        uploadFile(file);
    });

    // allow click on dropzone to open picker (удобно)
    drop.addEventListener("click", (e) => {
        // чтобы клик по ссылке/кнопке внутри dropzone не дублировался
        if (e.target && e.target.closest("button")) return;
        input.click();
    });
}

// --- Result rendering ---
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

    meta.textContent = `status=${report.status}, totals=${totals.length}, calls=${calls.length}`;

    // totals table
    totalsTBody.innerHTML = totals.map(t => {
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

    // calls table (optional)
    if (calls.length > 0) {
        callsTBody.innerHTML = calls.map(c => {
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

// --- Wire 3 endpoints ---
setupAutoUpload({
    inputId: "tariffsFile",
    dropId: "tariffsDrop",
    browseId: "tariffsBrowse",
    fileNameId: "tariffsFileName",
    progressId: "tariffsProgress",
    progressTextId: "tariffsProgressText",
    statusId: "tariffsStatus",
    urlBuilder: () => `${API_BASE}/api/v1/tariffs`,
    onSuccess: (_) => { /* ничего */ },
});

setupAutoUpload({
    inputId: "subsFile",
    dropId: "subsDrop",
    browseId: "subsBrowse",
    fileNameId: "subsFileName",
    progressId: "subsProgress",
    progressTextId: "subsProgressText",
    statusId: "subsStatus",
    urlBuilder: () => `${API_BASE}/api/v1/subscribers`,
    onSuccess: (_) => { /* ничего */ },
});

setupAutoUpload({
    inputId: "cdrFile",
    dropId: "cdrDrop",
    browseId: "cdrBrowse",
    fileNameId: "cdrFileName",
    progressId: "cdrProgress",
    progressTextId: "cdrProgressText",
    statusId: "cdrStatus",
    urlBuilder: () => {
        const collect = qs("collectCalls").checked;
        return `${API_BASE}/api/v1/cdr/tariff?collect_calls=${collect ? "true" : "false"}`;
    },
    onSuccess: (json) => {
        // здесь сервер возвращает результат тарификации
        renderReport(json);
    },
});

// init empty
renderReport(null);