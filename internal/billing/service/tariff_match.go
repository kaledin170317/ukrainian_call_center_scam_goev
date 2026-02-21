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
	"context"
	"time"

	"ukrainian_call_center_scam_goev/internal/billing/model"
)

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
	cost += model.Money((int64(rule.RatePerMin) * int64(cdr.BillableSec)) / 60)
	return cost
}
