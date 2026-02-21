// Copyright (c) 2023-2026, KNS Group LLC ("YADRO").
// All Rights Reserved.
// This software contains the intellectual property of YADRO
// or is licensed to YADRO from third parties. Use of this
// software and the intellectual property contained therein is expressly
// limited to the terms and conditions of the License Agreement under which
// it is provided by YADRO.
//

package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ukrainian_call_center_scam_goev/internal/service"
)

// Handler — тонкий слой: парсит запрос, вызывает сервис, маппит доменные данные в DTO.
type Handler struct {
	svc *billing.Service
}

func NewHandler(svc *billing.Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes — минимальный роутинг на стандартном ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/tariffs", h.uploadTariffs)
	mux.HandleFunc("POST /api/v1/subscribers", h.uploadSubscribers)
	mux.HandleFunc("POST /api/v1/cdr/tariff", h.tariffCDRStream)

	// необязательно, но удобно
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, OKResponse{Status: "ok"})
	})
}

// ---------- endpoints ----------

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

	// Если клиент передаёт Content-Length, можно прокинуть в сервис (для прогресса).
	totalBytes := int64(0)
	if r.ContentLength > 0 {
		totalBytes = r.ContentLength
	}

	report, err := h.svc.TariffCDRStream(ctx, reader, billing.CDRTariffOptions{
		CollectCalls:  collectCalls,
		TotalBytes:    totalBytes,
		ProgressEvery: 0,
		OnProgress:    nil, // позже можно сделать SSE, если нужно
	})
	if err != nil {
		code := "tariff_cdr_failed"
		status := http.StatusUnprocessableEntity
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			code = "request_canceled"
			status = 499 // nginx style; можно заменить на 408/499 как тебе надо
		}
		writeErr(w, status, code, err.Error())
		return
	}

	// Маппинг доменного отчёта -> DTO
	resp := TariffCDRResponse{
		Status: "ok",
		Totals: make([]SubscriberTotalDTO, 0, len(report.Totals)),
	}

	for _, t := range report.Totals {
		resp.Totals = append(resp.Totals, SubscriberTotalDTO{
			PhoneNumber:  t.PhoneNumber,
			ClientName:   t.ClientName,
			TotalCostKop: int64(t.TotalCost),
			CallsCount:   t.CallsCount,
		})
	}

	if collectCalls {
		resp.Calls = make([]RatedCallDTO, 0, len(report.Calls))
		for _, c := range report.Calls {
			var tr *AppliedTariffRefDTO
			if c.Tariff != nil {
				tr = &AppliedTariffRefDTO{
					Prefix:      c.Tariff.Prefix,
					Destination: c.Tariff.Destination,
					Priority:    c.Tariff.Priority,
				}
			}

			resp.Calls = append(resp.Calls, RatedCallDTO{
				StartTime:     c.StartTime.Format(time.RFC3339),
				EndTime:       c.EndTime.Format(time.RFC3339),
				CallingParty:  c.CallingParty,
				CalledParty:   c.CalledParty,
				CallDirection: c.CallDirection,
				Disposition:   c.Disposition,
				Duration:      c.Duration,
				BillableSec:   c.BillableSec,
				AccountCode:   c.AccountCode,
				CallID:        c.CallID,
				TrunkName:     c.TrunkName,
				CostKop:       int64(c.Cost),
				Tariff:        tr,
			})
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ---------- helpers ----------

// getUploadReader поддерживает:
// 1) multipart/form-data с полем file (и читает его как поток)
// 2) raw body (application/octet-stream / text/plain и т.п.)
func getUploadReader(r *http.Request, fieldName string) (io.Reader, io.Closer, error) {
	ct := r.Header.Get("Content-Type")
	mediatype, params, _ := mime.ParseMediaType(ct)

	if strings.EqualFold(mediatype, "multipart/form-data") {
		boundary := params["boundary"]
		if boundary == "" {
			return nil, nil, errors.New("multipart boundary is missing")
		}
		mr := multipart.NewReader(r.Body, boundary)

		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, nil, err
			}
			// form field name
			if part.FormName() == fieldName {
				// part нужно закрыть, но он будет читаться потоково,
				// поэтому возвращаем его как io.ReadCloser.
				return part, part, nil
			}
			part.Close()
		}
		return nil, nil, errors.New("file part not found")
	}

	// не multipart — читаем весь body как поток
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
	_ = json.NewEncoder(w).Encode(v)
}
