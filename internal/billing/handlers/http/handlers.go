// Copyright (c) 2023-2026, KNS Group LLC ("YADRO").
// All Rights Reserved.
// This software contains the intellectual property of YADRO
// or is licensed to YADRO from third parties. Use of this
// software and the intellectual property contained therein is expressly
// limited to the terms and conditions of the License Agreement under which
// it is provided by YADRO.

package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"ukrainian_call_center_scam_goev/internal/billing/model"
	billing "ukrainian_call_center_scam_goev/internal/billing/service"
)

type Handler struct {
	svc      *billing.Service
	progress *ProgressStore
	prepared *PreparedCDRStore
}

func NewHandler(svc *billing.Service) (*Handler, error) {
	prepared, err := NewPreparedCDRStore("", 2*time.Hour)
	if err != nil {
		return nil, err
	}

	return &Handler{
		svc:      svc,
		progress: NewProgressStore(),
		prepared: prepared,
	}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/tariffs", h.uploadTariffs)
	mux.HandleFunc("POST /api/v1/subscribers", h.uploadSubscribers)
	mux.HandleFunc("POST /api/v1/cdr/prepare", h.prepareCDR)
	mux.HandleFunc("POST /api/v1/cdr/start", h.startPreparedCDR)
	mux.HandleFunc("POST /api/v1/cdr/tariff", h.tariffCDRStream)
	mux.HandleFunc("GET /api/v1/cdr/progress/{id}", h.getCDRProgress)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, OKResponse{Status: "ok"})
	})
}

func (h *Handler) uploadTariffs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	reader, closer, _, err := getUploadSource(r, "file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	if closer != nil {
		defer closer.Close()
	}

	if err := h.svc.LoadTariffs(ctx, reader); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "load_tariffs_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, UploadResponse{Status: "ok"})
}

func (h *Handler) uploadSubscribers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	reader, closer, _, err := getUploadSource(r, "file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	if closer != nil {
		defer closer.Close()
	}

	if err := h.svc.LoadSubscribers(ctx, reader); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "load_subscribers_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, UploadResponse{Status: "ok"})
}

func (h *Handler) prepareCDR(w http.ResponseWriter, r *http.Request) {
	reader, closer, fileName, err := getUploadSource(r, "file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	if closer != nil {
		defer closer.Close()
	}

	meta, err := h.prepared.SaveNormalized(reader, fileName)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "prepare_cdr_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, PreparedCDRResponse{
		Status:          "ok",
		PreparedID:      meta.ID,
		FileName:        meta.OriginalName,
		RowsCount:       meta.RowsCount,
		NormalizedBytes: meta.NormalizedBytes,
	})
}

func (h *Handler) startPreparedCDR(w http.ResponseWriter, r *http.Request) {
	var req StartPreparedCDRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	req.PreparedID = strings.TrimSpace(req.PreparedID)
	req.ProgressID = strings.TrimSpace(req.ProgressID)
	if req.PreparedID == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "prepared_id is required")
		return
	}

	meta, ok := h.prepared.Get(req.PreparedID)
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "prepared file not found or expired")
		return
	}

	f, err := os.Open(meta.Path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "open_prepared_failed", err.Error())
		return
	}
	defer f.Close()

	report, calcMS, err := h.runTariffing(r.Context(), f, req.CollectCalls, req.ProgressID, meta.NormalizedBytes)
	if err != nil {
		h.writeTariffErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, buildTariffResponse(report, req.CollectCalls, calcMS))
}

func (h *Handler) tariffCDRStream(w http.ResponseWriter, r *http.Request) {
	reader, closer, _, err := getUploadSource(r, "file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	if closer != nil {
		defer closer.Close()
	}

	collectCalls := parseBoolQuery(r, "collect_calls", false)
	progressID := strings.TrimSpace(r.URL.Query().Get("progress_id"))

	totalBytes := parseInt64Query(r, "total_bytes", 0)
	if totalBytes <= 0 && r.ContentLength > 0 {
		totalBytes = r.ContentLength
	}

	report, calcMS, err := h.runTariffing(r.Context(), reader, collectCalls, progressID, totalBytes)
	if err != nil {
		h.writeTariffErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, buildTariffResponse(report, collectCalls, calcMS))
}

func (h *Handler) runTariffing(
	ctx context.Context,
	reader io.Reader,
	collectCalls bool,
	progressID string,
	totalBytes int64,
) (model.Report, float64, error) {
	var onProcessed func(int64)
	if progressID != "" {
		h.progress.Start(progressID, totalBytes)
		onProcessed = func(n int64) {
			h.progress.Add(progressID, int(n))
		}
	}

	started := time.Now()
	report, err := h.svc.TariffCDRStream(ctx, reader, model.Options{
		CollectCalls:     collectCalls,
		TotalBytes:       totalBytes,
		OnProcessedBytes: onProcessed,
		DemoSleepPerLine: 0,
	})
	calcMS := float64(time.Since(started).Microseconds()) / 1000

	if err != nil {
		if progressID != "" {
			h.progress.Fail(progressID, err)
		}
		return model.Report{}, calcMS, err
	}

	if progressID != "" {
		h.progress.Done(progressID)
	}

	return report, calcMS, nil
}

func buildTariffResponse(report model.Report, collectCalls bool, calcMS float64) TariffCDRResponse {
	resp := TariffCDRResponse{
		Status:        "ok",
		CalculationMS: calcMS,
		Totals:        mapTotals(report.Totals),
	}
	if collectCalls {
		resp.Calls = mapCalls(report.Calls)
	}

	return resp
}

func (h *Handler) writeTariffErr(w http.ResponseWriter, err error) {
	code := "tariff_cdr_failed"
	status := http.StatusUnprocessableEntity

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		code = "request_canceled"
		status = 499
	}

	writeErr(w, status, code, err.Error())
}

func (h *Handler) getCDRProgress(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "empty progress id")
		return
	}

	snap, ok := h.progress.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "progress id not found")
		return
	}

	writeJSON(w, http.StatusOK, snap)
}

func mapTotals(in []model.SubscriberTotal) []SubscriberTotalDTO {
	out := make([]SubscriberTotalDTO, 0, len(in))
	for _, t := range in {
		out = append(out, SubscriberTotalDTO{
			PhoneNumber:  string(t.PhoneNumber),
			ClientName:   t.ClientName,
			TotalCostKop: int64(t.TotalCost),
			CallsCount:   t.CallsCount,
		})
	}

	return out
}

func mapCalls(in []model.RatedCall) []RatedCallDTO {
	out := make([]RatedCallDTO, 0, len(in))
	for _, c := range in {
		var tr *AppliedTariffRefDTO
		if c.Tariff != nil {
			tr = &AppliedTariffRefDTO{
				Prefix:      c.Tariff.Prefix,
				Destination: c.Tariff.Destination,
				Priority:    c.Tariff.Priority,
			}
		}

		out = append(out, RatedCallDTO{
			StartTime:     c.StartTime.Format(time.RFC3339),
			EndTime:       c.EndTime.Format(time.RFC3339),
			CallingParty:  string(c.CallingParty),
			CalledParty:   string(c.CalledParty),
			CallDirection: c.Direction.String(),
			Disposition:   c.Disposition.String(),
			Duration:      c.Duration,
			BillableSec:   c.BillableSec,
			AccountCode:   c.AccountCode,
			CallID:        c.CallID,
			TrunkName:     c.TrunkName,
			CostKop:       int64(c.Cost),
			Tariff:        tr,
		})
	}

	return out
}

func getUploadSource(r *http.Request, fieldName string) (io.Reader, io.Closer, string, error) {
	ct := r.Header.Get("Content-Type")

	if strings.HasPrefix(strings.ToLower(ct), "multipart/form-data") {
		mr, err := r.MultipartReader()
		if err != nil {
			return nil, nil, "", err
		}

		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, nil, "", err
			}
			if part.FormName() == fieldName {
				return part, part, part.FileName(), nil
			}
			_ = part.Close()
		}

		return nil, nil, "", errors.New("file part not found")
	}

	if r.Body == nil {
		return nil, nil, "", errors.New("empty body")
	}

	return r.Body, r.Body, "upload", nil
}

func parseBoolQuery(r *http.Request, key string, def bool) bool {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return def
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}

	return b
}

func parseInt64Query(r *http.Request, key string, def int64) int64 {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return def
	}

	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}

	return n
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, ErrorResponse{
		Error: ErrorBody{
			Code:    code,
			Message: msg,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
