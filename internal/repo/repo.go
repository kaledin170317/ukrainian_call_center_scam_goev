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
	"time"

	"ukrainian_call_center_scam_goev/internal/model"
)

// SubscriberRepository хранит абонентскую базу и даёт быстрый доступ по номеру.
// CDR сюда не попадает вообще.
type SubscriberRepository interface {
	// ReplaceAll полностью заменяет данные (удобно при старте сервиса или при перезагрузке файлов).
	ReplaceAll(ctx context.Context, subs []model.Subscriber) error

	// GetByPhone ищет абонента по номеру (ключ = PhoneNumber).
	// ok=false если абонент не найден.
	GetByPhone(ctx context.Context, phone string) (sub model.Subscriber, ok bool, err error)
}

// TariffRepository хранит тарифные правила и умеет подбирать "лучшее" правило под звонок.
// Подбор должен быть максимально быстрым (для in-memory реализации обычно строится индекс по префиксам).
type TariffRepository interface {
	// ReplaceAll полностью заменяет набор тарифов (и пересобирает индексы).
	ReplaceAll(ctx context.Context, rules []model.TariffRule) error

	// MatchBest подбирает лучшее правило под номер назначения и момент времени.
	// number — номер, по которому матчим префикс (обычно CalledParty, но сервис решает сам).
	// at — момент времени звонка (обычно StartTime).
	//
	// repo внутри должно учитывать:
	// - prefix match (номер начинается с Prefix)
	// - effective/expiry date
	// - weekday
	// - timeband
	// - priority (и любые tie-breaker правила, если нужно)
	MatchBest(ctx context.Context, number string, at time.Time) (rule model.TariffRule, ok bool, err error)
}
