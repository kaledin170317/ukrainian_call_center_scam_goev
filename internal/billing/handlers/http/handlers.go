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
	"strconv"
	"strings"
	"time"

	"ukrainian_call_center_scam_goev/internal/billing/model"
	billing "ukrainian_call_center_scam_goev/internal/billing/service"
)

type Handler struct {
	svc *billing.Service
}

func NewHandler(svc *billing.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Go 1.22+ patterns
	mux.HandleFunc("POST /api/v1/tariffs", h.uploadTariffs)
	mux.HandleFunc("POST /api/v1/subscribers", h.uploadSubscribers)
	mux.HandleFunc("POST /api/v1/cdr/tariff", h.tariffCDRStream)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, OKResponse{Status: "ok"})
	})
}

func (h *Handler) uploadTariffs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	reader, closer, err := getUploadReader(r, "file")
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

	reader, closer, err := getUploadReader(r, "file")
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

func (h *Handler) tariffCDRStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	reader, closer, err := getUploadReader(r, "file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if closer != nil {
		defer closer.Close()
	}

	collectCalls := parseBoolQuery(r, "collect_calls", false)

	//totalBytes := int64(0)
	//if r.ContentLength > 0 {
	//	totalBytes = r.ContentLength
	//}

	report, err := h.svc.TariffCDRStream(ctx, reader, model.Options{
		CollectCalls: collectCalls,
		//TotalBytes:   totalBytes,
		// ProgressEvery/OnProgress можно подключить позже (SSE/WS), хендлер не меняется.
	})
	if err != nil {
		code := "tariff_cdr_failed"
		status := http.StatusUnprocessableEntity

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			code = "request_canceled"
			status = 499
		}

		writeErr(w, status, code, err.Error())
		return
	}

	resp := TariffCDRResponse{
		Status: "ok",
		Totals: mapTotals(report.Totals),
	}
	if collectCalls {
		resp.Calls = mapCalls(report.Calls)
	}

	writeJSON(w, http.StatusOK, resp)
}

// ===== mapping доменных структур в DTO =====

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

// ===== helpers =====

func getUploadReader(r *http.Request, fieldName string) (io.Reader, io.Closer, error) {
	ct := r.Header.Get("Content-Type")

	// multipart/form-data (из браузера)
	if strings.HasPrefix(strings.ToLower(ct), "multipart/form-data") {
		mr, err := r.MultipartReader()
		if err != nil {
			return nil, nil, err
		}
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, nil, err
			}
			if part.FormName() == fieldName {
				return part, part, nil
			}
			_ = part.Close()
		}
		return nil, nil, errors.New("file part not found")
	}

	// raw body (если решишь слать напрямую без multipart)
	if r.Body == nil {
		return nil, nil, errors.New("empty body")
	}
	return r.Body, r.Body, nil
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
