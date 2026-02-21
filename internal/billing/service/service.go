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
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"ukrainian_call_center_scam_goev/internal/billing/model"
	"ukrainian_call_center_scam_goev/internal/billing/repo"
)

const (
	cdrLayout  = "2006-01-02 15:04:05"
	dateLayout = "2006-01-02"
)

type Options struct {
	CollectCalls bool
}

type Service struct {
	tariffs repo.TariffRepository
	subs    repo.SubscriberRepository
	loc     *time.Location
}

func New(tariffs repo.TariffRepository, subs repo.SubscriberRepository) *Service {
	return &Service{
		tariffs: tariffs,
		subs:    subs,
		loc:     time.Local,
	}
}

// tariffs.csv (по скрину): 8 полей
// prefix;destination;rate_per_min;connection_fee;timeband;weekday;priority;effective_date
func (s *Service) LoadTariffs(ctx context.Context, r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	fields := make([]string, 8)
	var rules []model.TariffRule

	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		if !splitExact(line, ';', fields) {
			return fmt.Errorf("tariffs: expected 8 fields: %q", line)
		}
		for i := range fields {
			fields[i] = unquoteLoose(fields[i])
		}

		// header
		if first && strings.EqualFold(fields[0], "prefix") {
			first = false
			continue
		}
		first = false

		rate, err := model.ParseMoney(fields[2])
		if err != nil {
			return fmt.Errorf("tariffs: bad rate_per_min %q: %w", fields[2], err)
		}
		conn, err := model.ParseMoney(fields[3])
		if err != nil {
			return fmt.Errorf("tariffs: bad connection_fee %q: %w", fields[3], err)
		}
		tb, err := model.ParseTimeband(fields[4])
		if err != nil {
			return fmt.Errorf("tariffs: bad timeband %q: %w", fields[4], err)
		}
		wd, err := model.ParseWeekdayMask(fields[5])
		if err != nil {
			return fmt.Errorf("tariffs: bad weekday %q: %w", fields[5], err)
		}
		priority, err := strconv.Atoi(fields[6])
		if err != nil {
			return fmt.Errorf("tariffs: bad priority %q: %w", fields[6], err)
		}
		eff, err := time.ParseInLocation(dateLayout, fields[7], s.loc)
		if err != nil {
			return fmt.Errorf("tariffs: bad effective_date %q: %w", fields[7], err)
		}

		effStart := startOfDay(eff)
		expExcl := time.Date(2100, 1, 1, 0, 0, 0, 0, s.loc)

		rules = append(rules, model.TariffRule{
			Prefix:          fields[0],
			Destination:     fields[1],
			RatePerMin:      rate,
			ConnectionFee:   conn,
			Timeband:        tb,
			WeekdayMask:     wd,
			Priority:        priority,
			EffectiveStart:  effStart,
			ExpiryExclusive: expExcl,
		})
	}

	if err := sc.Err(); err != nil {
		return fmt.Errorf("read tariffs: %w", err)
	}
	return s.tariffs.ReplaceAll(ctx, rules)
}

// subscribers.csv: 2 поля
// phone_number;client_name
func (s *Service) LoadSubscribers(ctx context.Context, r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	fields := make([]string, 2)
	var subs []model.Subscriber

	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if !splitExact(line, ';', fields) {
			return fmt.Errorf("subscribers: expected 2 fields: %q", line)
		}
		for i := range fields {
			fields[i] = unquoteLoose(fields[i])
		}

		if first && strings.EqualFold(fields[0], "phone_number") {
			first = false
			continue
		}
		first = false

		subs = append(subs, model.Subscriber{
			PhoneNumber: fields[0],
			ClientName:  fields[1],
		})
	}

	if err := sc.Err(); err != nil {
		return fmt.Errorf("read subscribers: %w", err)
	}
	return s.subs.ReplaceAll(ctx, subs)
}

// cdr.txt: 12 полей через |
// StartTime|EndTime|CallingParty|CalledParty|CallDirection|Disposition|Duration|BillableSec|Charge|AccountCode|CallID|TrunkName
func (s *Service) TariffCDRStream(ctx context.Context, r io.Reader, opt Options) (model.Report, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	fields := make([]string, 12)

	totals := make(map[string]*model.SubscriberTotal, 1024)
	var calls []model.RatedCall
	if opt.CollectCalls {
		calls = make([]model.RatedCall, 0, 1024)
	}

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

		if !splitExact(line, '|', fields) {
			return model.Report{}, fmt.Errorf("cdr: expected 12 fields: %q", line)
		}
		for i := range fields {
			fields[i] = unquoteLoose(fields[i])
		}

		start, err := time.ParseInLocation(cdrLayout, fields[0], s.loc)
		if err != nil {
			return model.Report{}, fmt.Errorf("cdr: bad StartTime %q", fields[0])
		}
		end, err := time.ParseInLocation(cdrLayout, fields[1], s.loc)
		if err != nil {
			return model.Report{}, fmt.Errorf("cdr: bad EndTime %q", fields[1])
		}

		duration, err := atoiFast(fields[6])
		if err != nil {
			return model.Report{}, fmt.Errorf("cdr: bad Duration %q", fields[6])
		}
		bill, err := atoiFast(fields[7])
		if err != nil {
			return model.Report{}, fmt.Errorf("cdr: bad BillableSec %q", fields[7])
		}

		dir := model.ParseCallDirection(fields[4])
		disp := model.ParseDisposition(fields[5])

		cdr := model.CDRRecord{
			StartTime: start,
			EndTime:   end,

			CallingParty: fields[2],
			CalledParty:  fields[3],

			Direction:   dir,
			Disposition: disp,

			Duration:    duration,
			BillableSec: bill,

			AccountCode: fields[9],
			CallID:      fields[10],
			TrunkName:   fields[11],
		}

		subPhone := cdr.CallingParty
		sub, ok, err := s.subs.GetByPhone(ctx, subPhone)
		if err != nil {
			return model.Report{}, err
		}
		if !ok {
			sub = model.Subscriber{PhoneNumber: subPhone}
		}

		var best *model.TariffRule
		var cost model.Money

		// по сути тарифицируем только outgoing
		if cdr.Direction == model.DirOutgoing {
			best = s.matchBestTariff(ctx, cdr.CalledParty, cdr.StartTime)
			cost = calcCost(cdr, best)
		}

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

		if opt.CollectCalls {
			var ref *model.AppliedTariffRef
			if best != nil {
				ref = &model.AppliedTariffRef{
					Prefix:      best.Prefix,
					Destination: best.Destination,
					Priority:    best.Priority,
				}
			}
			calls = append(calls, model.RatedCall{
				StartTime:    cdr.StartTime,
				EndTime:      cdr.EndTime,
				CallingParty: cdr.CallingParty,
				CalledParty:  cdr.CalledParty,
				Direction:    cdr.Direction,
				Disposition:  cdr.Disposition,
				Duration:     cdr.Duration,
				BillableSec:  cdr.BillableSec,
				AccountCode:  cdr.AccountCode,
				CallID:       cdr.CallID,
				TrunkName:    cdr.TrunkName,
				Cost:         cost,
				Tariff:       ref,
			})
		}
	}

	if err := sc.Err(); err != nil {
		return model.Report{}, fmt.Errorf("read cdr: %w", err)
	}

	outTotals := make([]model.SubscriberTotal, 0, len(totals))
	for _, v := range totals {
		outTotals = append(outTotals, *v)
	}
	sort.Slice(outTotals, func(i, j int) bool {
		return outTotals[i].PhoneNumber < outTotals[j].PhoneNumber
	})

	return model.Report{Calls: calls, Totals: outTotals}, nil
}

func (s *Service) matchBestTariff(ctx context.Context, called string, at time.Time) *model.TariffRule {
	atMin := at.Hour()*60 + at.Minute()
	wd := weekdayBit(at.Weekday())

	var best *model.TariffRule
	bestPriority := -1
	bestPrefixLen := -1

	_ = s.tariffs.VisitByNumber(ctx, called, func(rule *model.TariffRule, prefixLen int) bool {
		if !isApplicable(rule, at, atMin, wd) {
			return true
		}
		if rule.Priority > bestPriority || (rule.Priority == bestPriority && prefixLen > bestPrefixLen) {
			best = rule
			bestPriority = rule.Priority
			bestPrefixLen = prefixLen
		}
		return true
	})

	return best
}

func isApplicable(rule *model.TariffRule, at time.Time, atMin int, wd uint8) bool {
	if at.Before(rule.EffectiveStart) || !at.Before(rule.ExpiryExclusive) {
		return false
	}
	if rule.WeekdayMask != 0 && (rule.WeekdayMask&(1<<wd)) == 0 {
		return false
	}

	// timeband: обычный или через полночь
	a := rule.Timeband.StartMin
	b := rule.Timeband.EndMin
	if a < b {
		return atMin >= a && atMin < b
	}
	return atMin >= a || atMin < b
}

func calcCost(cdr model.CDRRecord, rule *model.TariffRule) model.Money {
	if rule == nil {
		return 0
	}

	var cost model.Money
	if cdr.Disposition == model.DispAnswered {
		cost += rule.ConnectionFee
	}
	// копейки/мин * сек / 60
	cost += model.Money((int64(rule.RatePerMin) * int64(cdr.BillableSec)) / 60)
	return cost
}

func weekdayBit(w time.Weekday) uint8 {
	if w == time.Sunday {
		return 7
	}
	return uint8(w)
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func atoiFast(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty int")
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("bad int %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
