// Copyright (c) 2023-2026, KNS Group LLC ("YADRO").
// All Rights Reserved.
// This software contains the intellectual property of YADRO
// or is licensed to YADRO from third parties. Use of this
// software and the intellectual property contained therein is expressly
// limited to the terms and conditions of the License Agreement under which
// it is provided by YADRO.
//

package repo

import (
	"context"

	"ukrainian_call_center_scam_goev/internal/billing/model"
)

type SubscriberRepository interface {
	ReplaceAll(ctx context.Context, subs []model.Subscriber) error
	GetByPhone(ctx context.Context, phone string) (model.Subscriber, bool, error)
}

type TariffRepository interface {
	ReplaceAll(ctx context.Context, rules []model.TariffRule) error
	VisitByNumber(ctx context.Context, number string, visit func(rule *model.TariffRule, prefixLen int) bool) error
}
