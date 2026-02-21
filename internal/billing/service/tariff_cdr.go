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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ukrainian_call_center_scam_goev/internal/billing/model"
)

// cdr.txt: 12 полей через |
// StartTime|EndTime|CallingParty|CalledParty|CallDirection|Disposition|Duration|BillableSec|Charge|AccountCode|CallID|TrunkName

type cdrJob struct {
	ctx   context.Context
	batch *cdrBatch
	seq   uint64
	cdr   model.CDRRecord
}

type ratedCallSeq struct {
	seq  uint64
	call model.RatedCall
}

// cdrBatch holds per-call state for TariffCDRStream.
// It is updated concurrently by background workers.
type cdrBatch struct {
	collectCalls bool

	cancel     context.CancelFunc
	cancelOnce sync.Once

	mu     sync.Mutex
	totals map[string]*model.SubscriberTotal
	calls  []ratedCallSeq

	readingDone atomic.Bool
	pending     int64

	errMu sync.Mutex
	err   error

	doneOnce sync.Once
	done     chan struct{}
}

func newCDRBatch(collectCalls bool) *cdrBatch {
	b := &cdrBatch{
		collectCalls: collectCalls,
		totals:       make(map[string]*model.SubscriberTotal, 1024),
		done:         make(chan struct{}),
	}
	if collectCalls {
		b.calls = make([]ratedCallSeq, 0, 1024)
	}
	return b
}

func (b *cdrBatch) setErr(err error) {
	if err == nil {
		return
	}
	b.errMu.Lock()
	if b.err == nil {
		b.err = err
		b.errMu.Unlock()
		b.cancelOnce.Do(func() {
			if b.cancel != nil {
				b.cancel()
			}
		})
		return
	}
	b.errMu.Unlock()
}

func (b *cdrBatch) getErr() error {
	b.errMu.Lock()
	err := b.err
	b.errMu.Unlock()
	return err
}

func (b *cdrBatch) incPending() {
	atomic.AddInt64(&b.pending, 1)
}

func (b *cdrBatch) finishOne() {
	if atomic.AddInt64(&b.pending, -1) == 0 && b.readingDone.Load() {
		b.doneOnce.Do(func() { close(b.done) })
	}
}

func (b *cdrBatch) markReadingDone() {
	b.readingDone.Store(true)
	if atomic.LoadInt64(&b.pending) == 0 {
		b.doneOnce.Do(func() { close(b.done) })
	}
}

func (b *cdrBatch) add(sub model.Subscriber, cdr model.CDRRecord, cost model.Money, best *model.TariffRule, seq uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	t := b.totals[sub.PhoneNumber]
	if t == nil {
		t = &model.SubscriberTotal{PhoneNumber: sub.PhoneNumber, ClientName: sub.ClientName}
		b.totals[sub.PhoneNumber] = t
	} else if t.ClientName == "" && sub.ClientName != "" {
		// если сначала встретили неизвестного, а позже подтянули имя
		t.ClientName = sub.ClientName
	}

	t.TotalCost += cost
	t.CallsCount++

	if !b.collectCalls {
		return
	}

	var ref *model.AppliedTariffRef
	if best != nil {
		ref = &model.AppliedTariffRef{Prefix: best.Prefix, Destination: best.Destination, Priority: best.Priority}
	}

	b.calls = append(b.calls, ratedCallSeq{
		seq: seq,
		call: model.RatedCall{
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
		},
	})
}

// TariffCDRStream reads CDR stream in the caller goroutine and enqueues parsed rows into
// a global in-service queue. Background workers (started in New) are always running
// and consume from this queue.
func (s *Service) TariffCDRStream(ctx context.Context, r io.Reader, opt model.Options) (model.Report, error) {
	if err := s.ensureOpen(); err != nil {
		return model.Report{}, err
	}

	// Separate ctx for jobs so we can cancel workers' work on parse errors.
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	batch := newCDRBatch(opt.CollectCalls)
	batch.cancel = cancel
	batch.cancel = cancel

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var seq uint64
	for sc.Scan() {

		if jobCtx.Err() != nil {
			batch.setErr(jobCtx.Err())
			break
		}

		fields := strings.Split(sc.Text(), "|")

		start, err := time.ParseInLocation(cdrLayout, fields[0], s.loc)
		if err != nil {
			batch.setErr(fmt.Errorf("cdr: bad StartTime %q: %w", fields[0], err))
			cancel()
			break
		}

		end, err := time.ParseInLocation(cdrLayout, fields[1], s.loc)
		if err != nil {
			batch.setErr(fmt.Errorf("cdr: bad EndTime %q: %w", fields[1], err))
			cancel()
			break
		}

		duration, err := atoiFast(fields[6])
		if err != nil {
			batch.setErr(fmt.Errorf("cdr: bad Duration %q: %w", fields[6], err))
			cancel()
			break
		}
		bill, err := atoiFast(fields[7])
		if err != nil {
			batch.setErr(fmt.Errorf("cdr: bad BillableSec %q: %w", fields[7], err))
			cancel()
			break
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

		batch.incPending()
		job := cdrJob{ctx: jobCtx, batch: batch, seq: seq, cdr: cdr}
		seq++

		select {
		case s.jobs <- job:
			// ok
		case <-jobCtx.Done():
			batch.setErr(jobCtx.Err())
			batch.finishOne() // compensate incPending
			break
		case <-s.stopCtx.Done():
			batch.setErr(fmt.Errorf("billing service stopped"))
			batch.finishOne()
			cancel()
			break
		}

		// if a worker already produced an error (repo, etc.) — stop feeding more
		if batch.getErr() != nil {
			break
		}
	}

	if err := sc.Err(); err != nil {
		batch.setErr(fmt.Errorf("read cdr: %w", err))
		cancel()
	}

	batch.markReadingDone()

	select {
	case <-batch.done:
		// ok
	case <-ctx.Done():
		batch.setErr(ctx.Err())
		return model.Report{}, ctx.Err()
	case <-s.stopCtx.Done():
		batch.setErr(fmt.Errorf("billing service stopped"))
		return model.Report{}, fmt.Errorf("billing service stopped")
	}

	if err := batch.getErr(); err != nil {
		return model.Report{}, err
	}

	// Build report.
	totals := make([]model.SubscriberTotal, 0, len(batch.totals))
	for _, v := range batch.totals {
		totals = append(totals, *v)
	}
	sort.Slice(totals, func(i, j int) bool { return totals[i].PhoneNumber < totals[j].PhoneNumber })

	var calls []model.RatedCall
	if opt.CollectCalls {
		sort.Slice(batch.calls, func(i, j int) bool { return batch.calls[i].seq < batch.calls[j].seq })
		calls = make([]model.RatedCall, len(batch.calls))
		for i := range batch.calls {
			calls[i] = batch.calls[i].call
		}
	}

	return model.Report{Calls: calls, Totals: totals}, nil
}
