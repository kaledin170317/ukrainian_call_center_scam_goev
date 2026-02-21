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
	"strings"
)

type Money int64 // копейки

// ParseMoney("1.80") => 180
func ParseMoney(s string) (Money, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	// поддержим "," как десятичный разделитель
	s = strings.ReplaceAll(s, ",", ".")

	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}

	var rub int64
	var kop int64
	var seenDot bool
	var fracDigits int

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			if seenDot {
				return 0, fmt.Errorf("money: bad %q", s)
			}
			seenDot = true
			continue
		}
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("money: bad %q", s)
		}
		d := int64(c - '0')
		if !seenDot {
			rub = rub*10 + d
		} else {
			if fracDigits < 2 {
				kop = kop*10 + d
				fracDigits++
			}
		}
	}

	if seenDot {
		if fracDigits == 0 {
			kop = 0
		} else if fracDigits == 1 {
			kop *= 10
		}
	}

	v := rub*100 + kop
	if neg {
		v = -v
	}
	return Money(v), nil
}
