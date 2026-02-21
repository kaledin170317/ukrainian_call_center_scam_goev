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
	"ukrainian_call_center_scam_goev/internal/billing/model"
)

func (s *Service) startWorkers() {
	s.wg.Add(s.cdrWorkers)
	for i := 0; i < s.cdrWorkers; i++ {
		go s.cdrWorker()
	}
}

func (s *Service) cdrWorker() {
	defer s.wg.Done()

	for {
		select {
		case <-s.stopCtx.Done():
			return
		case job := <-s.jobs:
			b := job.batch
			if job.ctx.Err() != nil {
				b.finishOne()
				continue
			}

			subPhone := job.cdr.CallingParty
			sub, ok, err := s.subs.GetByPhone(job.ctx, subPhone)
			if err != nil {
				b.setErr(err)
				b.finishOne()
				continue
			}
			if !ok {
				sub = model.Subscriber{PhoneNumber: subPhone}
			}

			var best *model.TariffRule
			var cost model.Money

			if job.cdr.Direction == model.DirOutgoing {
				best = s.matchBestTariff(job.ctx, job.cdr.CalledParty, job.cdr.StartTime)
				cost = calcCost(job.cdr, best)
			}

			b.add(sub, job.cdr, cost, best, job.seq)
			b.finishOne()
		}
	}
}
