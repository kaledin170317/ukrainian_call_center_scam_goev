// Copyright (c) 2023-2026, KNS Group LLC ("YADRO").
// All Rights Reserved.
// This software contains the intellectual property of YADRO
// or is licensed to YADRO from third parties. Use of this
// software and the intellectual property contained therein is expressly
// limited to the terms and conditions of the License Agreement under which
// it is provided by YADRO.
//

package model

import "strings"

type CallDirection uint8

const (
	DirUnknown CallDirection = iota
	DirIncoming
	DirOutgoing
	DirInternal
)

func ParseCallDirection(s string) CallDirection {
	s = strings.TrimSpace(s)
	switch s {
	case "incoming":
		return DirIncoming
	case "outgoing":
		return DirOutgoing
	case "internal":
		return DirInternal
	default:
		return DirUnknown
	}
}

func (d CallDirection) String() string {
	switch d {
	case DirIncoming:
		return "incoming"
	case DirOutgoing:
		return "outgoing"
	case DirInternal:
		return "internal"
	default:
		return "unknown"
	}
}

type Disposition uint8

const (
	DispUnknown Disposition = iota
	DispAnswered
	DispBusy
	DispNoAnswer
	DispFailed
)

func ParseDisposition(s string) Disposition {
	s = strings.TrimSpace(s)
	switch s {
	case "answered":
		return DispAnswered
	case "busy":
		return DispBusy
	case "no_answer":
		return DispNoAnswer
	case "failed":
		return DispFailed
	default:
		return DispUnknown
	}
}

func (d Disposition) String() string {
	switch d {
	case DispAnswered:
		return "answered"
	case DispBusy:
		return "busy"
	case DispNoAnswer:
		return "no_answer"
	case DispFailed:
		return "failed"
	default:
		return "unknown"
	}
}
