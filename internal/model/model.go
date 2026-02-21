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

// Money хранится целым числом (например, в копейках), чтобы не использовать float.
type Money int64

// ===== 1) CDR (структура файла со звонками) =====
// Поля 1-в-1 как в задании.
type CDRRecord struct {
	StartTime time.Time
	EndTime   time.Time

	CallingParty  string
	CalledParty   string
	CallDirection string // incoming / outgoing / internal
	Disposition   string // answered / busy / no_answer / failed

	Duration    int
	BillableSec int
	Charge      Money

	AccountCode string
	CallID      string
	TrunkName   string
}

// ===== 2) Tariffs (структура файла с тарифами) =====
// Поля 1-в-1 как в задании.
type TariffRule struct {
	Prefix        string
	Destination   string
	RatePerMin    Money
	ConnectionFee Money
	Timeband      string // например "08:00-20:00"
	Weekday       string // например "1-5"
	Priority      int

	EffectiveDate time.Time
	ExpiryDate    time.Time
}

// ===== 3) Subscribers (структура абонентской базы) =====
// Поля 1-в-1 как в задании.
type Subscriber struct {
	PhoneNumber string
	ClientName  string
}

// =======================
// Результаты тарификации
// =======================

// 1) Общая начисленная сумма за период на абонента
type SubscriberTotal struct {
	PhoneNumber string
	ClientName  string
	TotalCost   Money
	CallsCount  int
}

// Ссылка на применённый тариф (чтобы в UI можно было перейти к тарифу)
type AppliedTariffRef struct {
	Prefix      string
	Destination string
	Priority    int
}

// 2) Просмотр конкретной записи о звонке с вычисленной стоимостью
// + ссылка на конкретный применённый тариф
type RatedCall struct {
	// данные звонка (как в CDR)
	StartTime time.Time
	EndTime   time.Time

	CallingParty  string
	CalledParty   string
	CallDirection string
	Disposition   string

	Duration    int
	BillableSec int

	AccountCode string
	CallID      string
	TrunkName   string

	// результат тарификации
	Cost   Money
	Tariff *AppliedTariffRef // nil, если тариф не найден/не применён
}

// Итоговый “отчёт” для UI (минимально)
type Report struct {
	Totals []SubscriberTotal
	Calls  []RatedCall
}
