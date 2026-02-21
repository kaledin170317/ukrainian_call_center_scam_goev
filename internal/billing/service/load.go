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
	"strconv"
	"strings"
	"time"

	"ukrainian_call_center_scam_goev/internal/billing/model"
)

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
