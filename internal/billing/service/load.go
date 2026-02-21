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

const (
	tariffsHeader     = "prefix;destination;rate_per_min;connection_fee;timeband;weekday;priority;effective_date;expiry_date" //nolint:lll
	subscribersHeader = "phone_number;client_name"
)

func (s *Service) LoadTariffs(ctx context.Context, r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	rules := make([]model.TariffRule, 0)

	sc.Scan()

	if sc.Text() != tariffsHeader {
		return fmt.Errorf("expected tariffs header: %q, actual: %q", tariffsHeader, sc.Text())
	}

	for sc.Scan() {
		fields := strings.Split(sc.Text(), ";")

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

		exp, err := time.ParseInLocation(dateLayout, fields[8], s.loc)
		if err != nil {
			return fmt.Errorf("tariffs: bad effective_date %q: %w", fields[8], err)
		}

		rules = append(rules, model.TariffRule{
			Prefix:          fields[0],
			Destination:     fields[1],
			RatePerMin:      rate,
			ConnectionFee:   conn,
			Timeband:        tb,
			WeekdayMask:     wd,
			Priority:        priority,
			EffectiveStart:  eff,
			ExpiryExclusive: exp.Add(time.Hour * 24),
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
	subs := make([]model.Subscriber, 0)

	sc.Scan()

	if sc.Text() != subscribersHeader {
		return fmt.Errorf("expected tariffs header: %q, actual: %q", subscribersHeader, sc.Text())
	}

	for sc.Scan() {
		fields := strings.Split(sc.Text(), ";")
		subs = append(subs, model.Subscriber{PhoneNumber: fields[0], ClientName: fields[1]})
	}

	if err := sc.Err(); err != nil {
		return fmt.Errorf("read subscribers: %w", err)
	}

	return s.subs.ReplaceAll(ctx, subs)
}
