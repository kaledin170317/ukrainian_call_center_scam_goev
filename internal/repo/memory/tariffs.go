// Copyright (c) 2023-2026, KNS Group LLC ("YADRO").
// All Rights Reserved.
// This software contains the intellectual property of YADRO
// or is licensed to YADRO from third parties. Use of this
// software and the intellectual property contained therein is expressly
// limited to the terms and conditions of the License Agreement under which
// it is provided by YADRO.
//

package memory

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"ukrainian_call_center_scam_goev/internal/model"
)

type compiledRule struct {
	raw       model.TariffRule
	order     int
	prefixLen int

	// распарсенное из raw.Timeband (минуты от начала суток)
	startMin int
	endMin   int

	// распарсенное из raw.Weekday (битмаска дней 1..7, где 1=Mon ... 7=Sun)
	weekdayMask uint8

	// [effectiveStart, expiryExclusive)
	effectiveStart  time.Time
	expiryExclusive time.Time
}

type tariffSnapshot struct {
	maxPrefixLen int
	byPrefix     map[string][]int // prefix -> indices in rules
	rules        []compiledRule   // immutable
}

type TariffMemoryRepo struct {
	v atomic.Value // *tariffSnapshot (immutable)
}

func NewTariffMemoryRepo() *TariffMemoryRepo {
	r := &TariffMemoryRepo{}
	r.v.Store(&tariffSnapshot{
		maxPrefixLen: 0,
		byPrefix:     make(map[string][]int),
		rules:        nil,
	})
	return r
}

func (r *TariffMemoryRepo) ReplaceAll(ctx context.Context, rules []model.TariffRule) error {
	_ = ctx

	snap := &tariffSnapshot{
		byPrefix: make(map[string][]int),
		rules:    make([]compiledRule, 0, len(rules)),
	}

	for i, rule := range rules {
		cr, err := compileTariffRule(rule, i)
		if err != nil {
			return fmt.Errorf("compile tariff rule #%d (prefix=%q): %w", i, rule.Prefix, err)
		}

		if cr.prefixLen > snap.maxPrefixLen {
			snap.maxPrefixLen = cr.prefixLen
		}

		idx := len(snap.rules)
		snap.rules = append(snap.rules, cr)
		snap.byPrefix[rule.Prefix] = append(snap.byPrefix[rule.Prefix], idx)
	}

	r.v.Store(snap)
	return nil
}

func (r *TariffMemoryRepo) MatchBest(ctx context.Context, number string, at time.Time) (model.TariffRule, bool, error) {
	_ = ctx

	snap := r.v.Load().(*tariffSnapshot)
	if len(snap.rules) == 0 || len(number) == 0 {
		return model.TariffRule{}, false, nil
	}

	n := normalizeNumber(number)
	if n == "" {
		return model.TariffRule{}, false, nil
	}

	atMin := at.Hour()*60 + at.Minute()
	wdBit := weekdayBit(at.Weekday()) // 1..7 (Mon..Sun)

	bestIdx := -1
	var best model.TariffRule

	// идём по длине префикса от более специфичного к менее
	maxL := snap.maxPrefixLen
	if maxL > len(n) {
		maxL = len(n)
	}

	for l := maxL; l >= 1; l-- {
		prefix := n[:l]
		idxs := snap.byPrefix[prefix]
		if len(idxs) == 0 {
			continue
		}

		for _, idx := range idxs {
			cr := snap.rules[idx]
			if !cr.matches(at, atMin, wdBit) {
				continue
			}

			if bestIdx < 0 || better(cr.raw, best) {
				bestIdx = idx
				best = cr.raw
			}
		}

		// если уже нашли на самом длинном префиксе — можно не продолжать
		// (но только если правило выбора не требует сравнивать с более короткими префиксами)
		// В типичной тарификации длиннее префикс всегда не хуже, поэтому выходим:
		if bestIdx >= 0 {
			break
		}
	}

	if bestIdx < 0 {
		return model.TariffRule{}, false, nil
	}
	return best, true, nil
}

// ===== helpers =====

func (cr compiledRule) matches(at time.Time, atMin int, wdBit uint8) bool {
	// даты
	if at.Before(cr.effectiveStart) || !at.Before(cr.expiryExclusive) {
		return false
	}

	// weekday
	if cr.weekdayMask != 0 {
		if (cr.weekdayMask & (1 << wdBit)) == 0 {
			return false
		}
	}

	// timeband
	if cr.startMin == 0 && cr.endMin == 0 {
		// если вдруг пусто/не распарсилось как 0-0 — считаем "не активно"
		// (можешь поменять на always-active при желании)
		return false
	}

	if cr.startMin < cr.endMin {
		// обычный диапазон
		return atMin >= cr.startMin && atMin < cr.endMin
	}

	// диапазон через полночь (например 20:00-08:00)
	return atMin >= cr.startMin || atMin < cr.endMin
}

func compileTariffRule(rule model.TariffRule, order int) (compiledRule, error) {
	startMin, endMin, err := parseTimeband(rule.Timeband) // "08:00-20:00" :contentReference[oaicite:1]{index=1}
	if err != nil {
		return compiledRule{}, err
	}

	mask, err := parseWeekdayMask(rule.Weekday) // "1-5" :contentReference[oaicite:2]{index=2}
	if err != nil {
		return compiledRule{}, err
	}

	eff := startOfDay(rule.EffectiveDate)
	expExcl := startOfDay(rule.ExpiryDate).Add(24 * time.Hour)

	return compiledRule{
		raw:             rule,
		order:           order,
		prefixLen:       len(rule.Prefix),
		startMin:        startMin,
		endMin:          endMin,
		weekdayMask:     mask,
		effectiveStart:  eff,
		expiryExclusive: expExcl,
	}, nil
}

func better(a, b model.TariffRule) bool {
	// правило выбора: больше priority — лучше.
	// если одинаково: длиннее prefix — лучше (более специфично).
	// если одинаково: можно оставить как есть (стабильность задаётся порядком обхода/файла).
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if len(a.Prefix) != len(b.Prefix) {
		return len(a.Prefix) > len(b.Prefix)
	}
	return false
}

func normalizeNumber(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// в CDR CalledParty может быть "+7916..." :contentReference[oaicite:3]{index=3}
	if s[0] == '+' {
		return s[1:]
	}
	return s
}

func parseTimeband(s string) (startMin int, endMin int, err error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid timeband %q", s)
	}

	a, err := parseHHMM(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid timeband start: %w", err)
	}
	b, err := parseHHMM(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid timeband end: %w", err)
	}
	return a, b, nil
}

func parseHHMM(s string) (int, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid hh:mm %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("out of range hh:mm %q", s)
	}
	return h*60 + m, nil
}

// weekday формата "1-5", но поддержим и "1,3,5" на всякий. :contentReference[oaicite:4]{index=4}
func parseWeekdayMask(s string) (uint8, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty weekday")
	}

	var mask uint8
	items := strings.Split(s, ",")
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if strings.Contains(it, "-") {
			p := strings.SplitN(it, "-", 2)
			if len(p) != 2 {
				return 0, fmt.Errorf("invalid weekday range %q", it)
			}
			from, err := strconv.Atoi(strings.TrimSpace(p[0]))
			if err != nil {
				return 0, err
			}
			to, err := strconv.Atoi(strings.TrimSpace(p[1]))
			if err != nil {
				return 0, err
			}
			if from < 1 || from > 7 || to < 1 || to > 7 || from > to {
				return 0, fmt.Errorf("weekday range out of bounds %q", it)
			}
			for d := from; d <= to; d++ {
				mask |= 1 << uint8(d)
			}
			continue
		}

		d, err := strconv.Atoi(it)
		if err != nil {
			return 0, err
		}
		if d < 1 || d > 7 {
			return 0, fmt.Errorf("weekday out of bounds %q", it)
		}
		mask |= 1 << uint8(d)
	}

	if mask == 0 {
		return 0, fmt.Errorf("weekday mask is zero (%q)", s)
	}
	return mask, nil
}

func weekdayBit(w time.Weekday) uint8 {
	// time.Weekday: Sunday=0 ... Saturday=6
	// Нам нужно: Monday=1 ... Sunday=7
	if w == time.Sunday {
		return 7
	}
	return uint8(w) // Monday=1 ... Saturday=6
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	loc := t.Location()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}
