// Copyright (c) 2023-2026, KNS Group LLC ("YADRO").
// All Rights Reserved.
// This software contains the intellectual property of YADRO
// or is licensed to YADRO from third parties. Use of this
// software and the intellectual property contained therein is expressly
// limited to the terms and conditions of the License Agreement under which
// it is provided by YADRO.
//

package billing

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"ukrainian_call_center_scam_goev/internal/model"
	"ukrainian_call_center_scam_goev/internal/repo"
)

const (
	cdrDateTimeLayout = "2006-01-02 15:04:05" // StartTime/EndTime в CDR
	dateLayout        = "2006-01-02"          // effective_date/expiry_date
)

// Progress позволяет потом красиво показывать прогресс в UI.
// totalBytes можешь передать из Content-Length (если известен), иначе 0.
type Progress struct {
	LinesRead  int64
	BytesRead  int64
	TotalBytes int64
}

type CDRTariffOptions struct {
	// Если true — собираем Report.Calls (может быть много памяти).
	// Если false — считаем только Totals (наиболее экономно).
	CollectCalls bool

	// Как часто дёргать прогресс (в строках). 0 => дефолт 1000.
	ProgressEvery int64

	// Обратный вызов прогресса (может быть nil).
	OnProgress func(p Progress)

	// Если известен размер тела (например Content-Length) — передай сюда.
	TotalBytes int64
}

type Service struct {
	tariffs repo.TariffRepository
	subs    repo.SubscriberRepository
}

func New(tariffs repo.TariffRepository, subs repo.SubscriberRepository) *Service {
	return &Service{
		tariffs: tariffs,
		subs:    subs,
	}
}

// 1) Загрузка тарифов (CSV с разделителем ';')
func (s *Service) LoadTariffs(ctx context.Context, r io.Reader) error {
	cr := csv.NewReader(r)
	cr.Comma = ';'
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	var rules []model.TariffRule
	first := true

	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tariffs csv: %w", err)
		}
		if isEmptyCSVRow(rec) {
			continue
		}

		// возможная шапка: prefix;destination;rate_per_min;...
		if first && looksLikeHeader(rec, "prefix", "destination") {
			first = false
			continue
		}
		first = false

		// ожидаем 9 полей
		if len(rec) < 9 {
			return fmt.Errorf("tariffs: expected 9 fields, got %d: %v", len(rec), rec)
		}

		rate, err := parseMoney(rec[2])
		if err != nil {
			return fmt.Errorf("tariffs: bad rate_per_min %q: %w", rec[2], err)
		}
		conn, err := parseMoney(rec[3])
		if err != nil {
			return fmt.Errorf("tariffs: bad connection_fee %q: %w", rec[3], err)
		}
		priority, err := strconv.Atoi(strings.TrimSpace(rec[6]))
		if err != nil {
			return fmt.Errorf("tariffs: bad priority %q: %w", rec[6], err)
		}
		eff, err := time.Parse(dateLayout, strings.TrimSpace(rec[7]))
		if err != nil {
			return fmt.Errorf("tariffs: bad effective_date %q: %w", rec[7], err)
		}
		exp, err := time.Parse(dateLayout, strings.TrimSpace(rec[8]))
		if err != nil {
			return fmt.Errorf("tariffs: bad expiry_date %q: %w", rec[8], err)
		}

		rules = append(rules, model.TariffRule{
			Prefix:        strings.TrimSpace(rec[0]),
			Destination:   strings.TrimSpace(rec[1]),
			RatePerMin:    rate,
			ConnectionFee: conn,
			Timeband:      strings.TrimSpace(rec[4]),
			Weekday:       strings.TrimSpace(rec[5]),
			Priority:      priority,
			EffectiveDate: eff,
			ExpiryDate:    exp,
		})
	}

	return s.tariffs.ReplaceAll(ctx, rules)
}

// 2) Загрузка абонентов (CSV с разделителем ';')
func (s *Service) LoadSubscribers(ctx context.Context, r io.Reader) error {
	cr := csv.NewReader(r)
	cr.Comma = ';'
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	var subs []model.Subscriber
	first := true

	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read subscribers csv: %w", err)
		}
		if isEmptyCSVRow(rec) {
			continue
		}

		// возможная шапка: phone_number;client_name
		if first && looksLikeHeader(rec, "phone_number", "client_name") {
			first = false
			continue
		}
		first = false

		if len(rec) < 2 {
			return fmt.Errorf("subscribers: expected 2 fields, got %d: %v", len(rec), rec)
		}

		subs = append(subs, model.Subscriber{
			PhoneNumber: strings.TrimSpace(rec[0]),
			ClientName:  strings.TrimSpace(rec[1]),
		})
	}

	return s.subs.ReplaceAll(ctx, subs)
}

// 3) Тарификация CDR ПОТОКОМ (текст с разделителем '|')
// CDR нигде не сохраняем — читаем строку -> считаем -> обновляем totals (+опционально calls)
func (s *Service) TariffCDRStream(ctx context.Context, r io.Reader, opt CDRTariffOptions) (model.Report, error) {
	if opt.ProgressEvery <= 0 {
		opt.ProgressEvery = 1000
	}

	// считаем байты на лету (удобно для прогресса)
	counting := &countingReader{r: r}

	sc := bufio.NewScanner(counting)
	// на всякий случай увеличим буфер (CDR строки обычно короткие, но пусть будет)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	totals := make(map[string]*model.SubscriberTotal, 1024)

	var calls []model.RatedCall
	if opt.CollectCalls {
		calls = make([]model.RatedCall, 0, 1024)
	}

	var lines int64

	for sc.Scan() {
		select {
		case <-ctx.Done():
			return model.Report{}, ctx.Err()
		default:
		}

		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		cdr, err := parseCDRLine(line)
		if err != nil {
			return model.Report{}, fmt.Errorf("cdr parse error at line %d: %w", lines+1, err)
		}

		// Абонент = CallingParty (внутренний номер) из задания :contentReference[oaicite:1]{index=1}
		subPhone := cdr.CallingParty
		sub, ok, err := s.subs.GetByPhone(ctx, subPhone)
		if err != nil {
			return model.Report{}, fmt.Errorf("get subscriber %q: %w", subPhone, err)
		}
		if !ok {
			// если не нашли — считаем всё равно, но ClientName будет пустой
			sub = model.Subscriber{PhoneNumber: subPhone}
		}

		// Тариф подбираем по набранному номеру (CalledParty) и моменту начала вызова
		rule, ruleOK, err := s.tariffs.MatchBest(ctx, cdr.CalledParty, cdr.StartTime)
		if err != nil {
			return model.Report{}, fmt.Errorf("match tariff for %q: %w", cdr.CalledParty, err)
		}

		var cost model.Money
		var tariffRef *model.AppliedTariffRef
		if ruleOK {
			cost = calcCost(cdr, rule)
			tariffRef = &model.AppliedTariffRef{
				Prefix:      rule.Prefix,
				Destination: rule.Destination,
				Priority:    rule.Priority,
			}
		} else {
			cost = 0
			tariffRef = nil
		}

		// totals
		t := totals[sub.PhoneNumber]
		if t == nil {
			t = &model.SubscriberTotal{
				PhoneNumber: sub.PhoneNumber,
				ClientName:  sub.ClientName,
			}
			totals[sub.PhoneNumber] = t
		}
		t.TotalCost += cost
		t.CallsCount++

		// calls (опционально)
		if opt.CollectCalls {
			calls = append(calls, model.RatedCall{
				StartTime:     cdr.StartTime,
				EndTime:       cdr.EndTime,
				CallingParty:  cdr.CallingParty,
				CalledParty:   cdr.CalledParty,
				CallDirection: cdr.CallDirection,
				Disposition:   cdr.Disposition,
				Duration:      cdr.Duration,
				BillableSec:   cdr.BillableSec,
				AccountCode:   cdr.AccountCode,
				CallID:        cdr.CallID,
				TrunkName:     cdr.TrunkName,
				Cost:          cost,
				Tariff:        tariffRef,
			})
		}

		lines++
		if opt.OnProgress != nil && (lines%opt.ProgressEvery == 0) {
			opt.OnProgress(Progress{
				LinesRead:  lines,
				BytesRead:  counting.n,
				TotalBytes: opt.TotalBytes,
			})
		}
	}

	if err := sc.Err(); err != nil {
		return model.Report{}, fmt.Errorf("cdr scan: %w", err)
	}

	// финальный прогресс
	if opt.OnProgress != nil {
		opt.OnProgress(Progress{
			LinesRead:  lines,
			BytesRead:  counting.n,
			TotalBytes: opt.TotalBytes,
		})
	}

	// собираем totals в слайс (отсортируем для стабильности)
	outTotals := make([]model.SubscriberTotal, 0, len(totals))
	for _, v := range totals {
		outTotals = append(outTotals, *v)
	}
	sort.Slice(outTotals, func(i, j int) bool {
		return outTotals[i].PhoneNumber < outTotals[j].PhoneNumber
	})

	return model.Report{
		Calls:  calls,
		Totals: outTotals,
	}, nil
}

// ====== helpers ======

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func parseCDRLine(line string) (model.CDRRecord, error) {
	// CDR: 12 полей, разделитель '|' :contentReference[oaicite:2]{index=2}
	parts := strings.Split(line, "|")
	if len(parts) < 12 {
		return model.CDRRecord{}, fmt.Errorf("expected 12 fields, got %d", len(parts))
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	start, err := time.Parse(cdrDateTimeLayout, parts[0])
	if err != nil {
		return model.CDRRecord{}, fmt.Errorf("bad StartTime %q: %w", parts[0], err)
	}
	end, err := time.Parse(cdrDateTimeLayout, parts[1])
	if err != nil {
		return model.CDRRecord{}, fmt.Errorf("bad EndTime %q: %w", parts[1], err)
	}

	duration, err := atoi(parts[6])
	if err != nil {
		return model.CDRRecord{}, fmt.Errorf("bad Duration %q: %w", parts[6], err)
	}
	bill, err := atoi(parts[7])
	if err != nil {
		return model.CDRRecord{}, fmt.Errorf("bad BillableSec %q: %w", parts[7], err)
	}

	charge, err := parseMoney(parts[8]) // из CDR, может быть 0.45 :contentReference[oaicite:3]{index=3}
	if err != nil {
		return model.CDRRecord{}, fmt.Errorf("bad Charge %q: %w", parts[8], err)
	}

	return model.CDRRecord{
		StartTime: start,
		EndTime:   end,

		CallingParty:  parts[2],
		CalledParty:   parts[3],
		CallDirection: parts[4],
		Disposition:   parts[5],

		Duration:    duration,
		BillableSec: bill,
		Charge:      charge,

		AccountCode: parts[9],
		CallID:      parts[10],
		TrunkName:   parts[11],
	}, nil
}

// calcCost — полностью integer-арифметика, без float.
// ConnectionFee берём только если вызов установлен (answered) — по описанию тарифа. :contentReference[oaicite:4]{index=4}
func calcCost(cdr model.CDRRecord, rule model.TariffRule) model.Money {
	var cost model.Money

	if strings.EqualFold(cdr.Disposition, "answered") {
		cost += rule.ConnectionFee
	}

	// "rate_per_min" задан как стоимость минуты.
	// Считаем пропорционально BillableSec: rate_per_min * sec / 60.
	// Чтобы не терять копейки — округляем вверх по копейкам.
	// (Если позже в ТЗ будет другая политика — поменяешь одну строку.)
	secCost := (int64(rule.RatePerMin)*int64(cdr.BillableSec) + 59) / 60
	cost += model.Money(secCost)

	return cost
}

// parseMoney парсит строки вида "0.45", "1.80", "2", "2.5" в копейки (Money).
// Без float.
func parseMoney(s string) (model.Money, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	s = strings.ReplaceAll(s, ",", ".")
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = strings.TrimPrefix(s, "-")
	}

	parts := strings.SplitN(s, ".", 3)
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid money %q", s)
	}

	rub, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid rubles %q", parts[0])
	}

	var kop int64
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) == 1 {
			frac += "0"
		} else if len(frac) == 0 {
			frac = "00"
		} else if len(frac) > 2 {
			frac = frac[:2]
		}
		kop, err = strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid kopeks %q", parts[1])
		}
	}

	val := rub*100 + kop
	if neg {
		val = -val
	}
	return model.Money(val), nil
}

func atoi(s string) (int, error) {
	s = strings.TrimSpace(s)
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func isEmptyCSVRow(rec []string) bool {
	if len(rec) == 0 {
		return true
	}
	if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
		return true
	}
	allEmpty := true
	for _, x := range rec {
		if strings.TrimSpace(x) != "" {
			allEmpty = false
			break
		}
	}
	return allEmpty
}

func looksLikeHeader(rec []string, a, b string) bool {
	if len(rec) < 2 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(rec[0]), a) &&
		strings.EqualFold(strings.TrimSpace(rec[1]), b)
}
