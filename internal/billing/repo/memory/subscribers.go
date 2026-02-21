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
	"sync/atomic"

	"ukrainian_call_center_scam_goev/internal/billing/model"
)

type subSnap struct {
	byPhone map[string]model.Subscriber
}

type SubscriberMemoryRepo struct {
	v atomic.Value // *subSnap
}

func NewSubscriberMemoryRepo() *SubscriberMemoryRepo {
	r := &SubscriberMemoryRepo{}
	r.v.Store(&subSnap{byPhone: map[string]model.Subscriber{}})

	return r
}

func (r *SubscriberMemoryRepo) ReplaceAll(ctx context.Context, subs []model.Subscriber) error {
	_ = ctx

	m := make(map[string]model.Subscriber, len(subs))
	for _, s := range subs {
		m[s.PhoneNumber] = s
	}

	r.v.Store(&subSnap{byPhone: m})

	return nil
}

func (r *SubscriberMemoryRepo) GetByPhone(ctx context.Context, phone string) (model.Subscriber, bool, error) {
	_ = ctx

	s := r.v.Load().(*subSnap)
	sub, ok := s.byPhone[phone]

	return sub, ok, nil
}
