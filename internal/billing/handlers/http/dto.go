// Copyright (c) 2023-2026, KNS Group LLC ("YADRO").
// All Rights Reserved.
// This software contains the intellectual property of YADRO
// or is licensed to YADRO from third parties. Use of this
// software and the intellectual property contained therein is expressly
// limited to the terms and conditions of the License Agreement under which
// it is provided by YADRO.
//

package http

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type OKResponse struct {
	Status string `json:"status"`
}

type UploadResponse struct {
	Status string `json:"status"`
}

type SubscriberTotalDTO struct {
	PhoneNumber  string `json:"phone_number"`
	ClientName   string `json:"client_name,omitempty"`
	TotalCostKop int64  `json:"total_cost_kop"`
	CallsCount   int    `json:"calls_count"`
}

type AppliedTariffRefDTO struct {
	Prefix      string `json:"prefix"`
	Destination string `json:"destination"`
	Priority    int    `json:"priority"`
}

type RatedCallDTO struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`

	CallingParty  string `json:"calling_party"`
	CalledParty   string `json:"called_party"`
	CallDirection string `json:"call_direction"`
	Disposition   string `json:"disposition"`

	Duration    int `json:"duration"`
	BillableSec int `json:"billable_sec"`

	AccountCode string `json:"account_code,omitempty"`
	CallID      string `json:"call_id,omitempty"`
	TrunkName   string `json:"trunk_name,omitempty"`

	CostKop int64                `json:"cost_kop"`
	Tariff  *AppliedTariffRefDTO `json:"tariff,omitempty"`
}

type TariffCDRResponse struct {
	Status string               `json:"status"`
	Totals []SubscriberTotalDTO `json:"totals"`
	Calls  []RatedCallDTO       `json:"calls,omitempty"`
}
