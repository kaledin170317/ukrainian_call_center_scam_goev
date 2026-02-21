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
	"fmt"
	"strings"
	"time"
)

func weekdayBit(w time.Weekday) uint8 {
	if w == time.Sunday {
		return 7
	}

	return uint8(w)
}

func atoiFast(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty int")
	}

	n := 0

	for i := range len(s) {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("bad int %q", s)
		}

		n = n*10 + int(c-'0')
	}

	return n, nil
}
