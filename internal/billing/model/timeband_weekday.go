// Copyright (c) 2023-2026, KNS Group LLC ("YADRO").
// All Rights Reserved.
// This software contains the intellectual property of YADRO
// or is licensed to YADRO from third parties. Use of this
// software and the intellectual property contained therein is expressly
// limited to the terms and conditions of the License Agreement under which
// it is provided by YADRO.
//

package model

import (
	"fmt"
	"strconv"
	"strings"
)

type Timeband struct {
	StartMin int // включительно
	EndMin   int // эксклюзивно
}

// "08:00-20:00"
func ParseTimeband(s string) (Timeband, error) {
	s = strings.TrimSpace(s)
	p := strings.SplitN(s, "-", 2)
	if len(p) != 2 {
		return Timeband{}, fmt.Errorf("timeband: bad %q", s)
	}
	a, err := parseHHMM(p[0])
	if err != nil {
		return Timeband{}, err
	}
	b, err := parseHHMM(p[1])
	if err != nil {
		return Timeband{}, err
	}
	return Timeband{StartMin: a, EndMin: b}, nil
}

func parseHHMM(s string) (int, error) {
	s = strings.TrimSpace(s)
	p := strings.SplitN(s, ":", 2)
	if len(p) != 2 {
		return 0, fmt.Errorf("hhmm: bad %q", s)
	}
	h, err := strconv.Atoi(p[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(p[1])
	if err != nil {
		return 0, err
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("hhmm: out of range %q", s)
	}
	return h*60 + m, nil
}

// weekday: "1-5" или "1,3,5" или "1-7"
func ParseWeekdayMask(s string) (uint8, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("weekday: empty")
	}

	var mask uint8
	items := strings.Split(s, ",")
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" {
			continue
		}
		if strings.Contains(it, "-") {
			p := strings.SplitN(it, "-", 2)
			if len(p) != 2 {
				return 0, fmt.Errorf("weekday: bad range %q", it)
			}
			from, err := strconv.Atoi(strings.TrimSpace(p[0]))
			if err != nil {
				return 0, err
			}
			to, err := strconv.Atoi(strings.TrimSpace(p[1]))
			if err != nil {
				return 0, err
			}
			if from < 1 || from > 7 || to < 1 || to > 7 || from > to {
				return 0, fmt.Errorf("weekday: out of bounds %q", it)
			}
			for d := from; d <= to; d++ {
				mask |= 1 << uint8(d)
			}
			continue
		}

		d, err := strconv.Atoi(it)
		if err != nil {
			return 0, err
		}
		if d < 1 || d > 7 {
			return 0, fmt.Errorf("weekday: out of bounds %q", it)
		}
		mask |= 1 << uint8(d)
	}

	if mask == 0 {
		return 0, fmt.Errorf("weekday: zero mask")
	}
	return mask, nil
}
