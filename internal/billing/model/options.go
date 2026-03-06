// Copyright (c) 2023-2026, KNS Group LLC ("YADRO").
// All Rights Reserved.
// This software contains the intellectual property of YADRO
// or is licensed to YADRO from third parties. Use of this
// software and the intellectual property contained therein is expressly
// limited to the terms and conditions of the License Agreement under which
// it is provided by YADRO.
//

package model

import "time"

// Options configures stream tariffing.
type Options struct {
	CollectCalls bool
	TotalBytes   int64

	// OnProcessedBytes is called after a CDR row is fully processed (rated and accounted).
	// n is an approximate byte size of the processed row (used for progress UI).
	OnProcessedBytes func(n int64)

	// DemoSleepPerLine slows down processing for demo UI (set to 0 to disable).
	DemoSleepPerLine time.Duration
}
