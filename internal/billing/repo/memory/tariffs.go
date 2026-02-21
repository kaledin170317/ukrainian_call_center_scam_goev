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
	"strings"
	"sync/atomic"

	"ukrainian_call_center_scam_goev/internal/billing/model"
)

type tariffSnap struct {
	rules        []model.TariffRule
	byPrefix     map[string][]int
	maxPrefixLen int
}

type TariffMemoryRepo struct {
	v atomic.Value // *tariffSnap
}

func NewTariffMemoryRepo() *TariffMemoryRepo {
	r := &TariffMemoryRepo{}
	r.v.Store(&tariffSnap{byPrefix: map[string][]int{}})
	return r
}

func (r *TariffMemoryRepo) ReplaceAll(ctx context.Context, rules []model.TariffRule) error {
	_ = ctx

	rs := make([]model.TariffRule, len(rules))
	copy(rs, rules)

	byPrefix := make(map[string][]int, len(rs))
	maxL := 0

	for i := range rs {
		p := rs[i].Prefix
		byPrefix[p] = append(byPrefix[p], i)
		if len(p) > maxL {
			maxL = len(p)
		}
	}

	r.v.Store(&tariffSnap{
		rules:        rs,
		byPrefix:     byPrefix,
		maxPrefixLen: maxL,
	})
	return nil
}

func (r *TariffMemoryRepo) VisitByNumber(ctx context.Context, number string, visit func(rule *model.TariffRule, prefixLen int) bool) error {
	_ = ctx

	s := r.v.Load().(*tariffSnap)
	if len(s.rules) == 0 {
		return nil
	}

	n := normalizeNumber(number)
	if n == "" {
		return nil
	}

	maxL := s.maxPrefixLen
	if maxL > len(n) {
		maxL = len(n)
	}

	// Идём от длинного префикса к короткому
	for l := maxL; l >= 1; l-- {
		prefix := n[:l]
		idxs := s.byPrefix[prefix]
		if len(idxs) == 0 {
			continue
		}
		for _, idx := range idxs {
			if !visit(&s.rules[idx], l) {
				return nil
			}
		}
	}
	return nil
}

func normalizeNumber(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if s[0] == '+' {
		s = s[1:]
	}
	return s
}
