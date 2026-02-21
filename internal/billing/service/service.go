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
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"ukrainian_call_center_scam_goev/internal/billing/model"
	"ukrainian_call_center_scam_goev/internal/billing/repo"
)

const (
	cdrLayout  = "2006-01-02 15:04:05"
	dateLayout = "2006-01-02"
)

type Config struct {
	Location     *time.Location
	CDRWorkers   int
	CDRQueueSize int
}

type Service struct {
	tariffs repo.TariffRepository
	subs    repo.SubscriberRepository
	loc     *time.Location

	cdrWorkers int

	jobs chan cdrJob

	closeOnce sync.Once
	closed    atomic.Bool

	stopCtx    context.Context
	stopCancel context.CancelFunc
	wg         sync.WaitGroup
}

func New(
	tariffs repo.TariffRepository,
	subs repo.SubscriberRepository,
	location *time.Location,
	cdrWorkers int,
) *Service {
	s := &Service{
		tariffs:    tariffs,
		subs:       subs,
		loc:        location,
		cdrWorkers: cdrWorkers,
	}

	s.jobs = make(chan cdrJob)
	s.stopCtx, s.stopCancel = context.WithCancel(context.Background())
	s.startWorkers()

	return s
}

func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		s.stopCancel()
		s.wg.Wait()
	})
}

func (s *Service) ensureOpen() error {
	if s.closed.Load() {
		return fmt.Errorf("billing service is closed")
	}

	return nil
}

func (s *Service) startWorkers() {
	s.wg.Add(s.cdrWorkers)

	for range s.cdrWorkers {
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

			var (
				best *model.TariffRule
				cost model.Money
			)

			if job.cdr.Direction == model.DirOutgoing {
				best = s.matchBestTariff(job.ctx, job.cdr.CalledParty, job.cdr.StartTime)
				cost = calcCost(job.cdr, best)
			}

			b.add(sub, job.cdr, cost, best, job.seq)
			b.finishOne()
		}
	}
}
