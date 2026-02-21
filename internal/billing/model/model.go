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

type Subscriber struct {
	PhoneNumber string
	ClientName  string
}

type TariffRule struct {
	Prefix        string
	Destination   string
	RatePerMin    Money
	ConnectionFee Money
	Timeband      Timeband
	WeekdayMask   uint8
	Priority      int

	EffectiveStart  time.Time
	ExpiryExclusive time.Time
}

type AppliedTariffRef struct {
	Prefix      string
	Destination string
	Priority    int
}

type CDRRecord struct {
	StartTime time.Time
	EndTime   time.Time

	CallingParty string
	CalledParty  string

	Direction   CallDirection
	Disposition Disposition

	Duration    int
	BillableSec int

	AccountCode string
	CallID      string
	TrunkName   string
}

type RatedCall struct {
	StartTime time.Time
	EndTime   time.Time

	CallingParty string
	CalledParty  string

	Direction   CallDirection
	Disposition Disposition

	Duration    int
	BillableSec int

	AccountCode string
	CallID      string
	TrunkName   string

	Cost   Money
	Tariff *AppliedTariffRef
}

type SubscriberTotal struct {
	PhoneNumber string
	ClientName  string
	TotalCost   Money
	CallsCount  int
}

type Report struct {
	Calls  []RatedCall
	Totals []SubscriberTotal
}
