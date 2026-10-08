// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package dogetx

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/DogecoinVM/dogevm-connect/internal/metalapi"
)

const KoinuPerDoge int64 = 100_000_000

// ParseDOGE turns a decimal DOGE string into koinu.
func ParseDOGE(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty amount")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	whole := parts[0]
	if whole == "" {
		whole = "0"
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if len(frac) > 8 {
		return 0, fmt.Errorf("too many decimal places")
	}
	for len(frac) < 8 {
		frac += "0"
	}
	w := new(big.Int)
	if _, ok := w.SetString(whole, 10); !ok {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	f := new(big.Int)
	if _, ok := f.SetString(frac, 10); !ok {
		return 0, fmt.Errorf("invalid amount %q", s)
	}
	w.Mul(w, big.NewInt(KoinuPerDoge))
	w.Add(w, f)
	if !w.IsInt64() {
		return 0, fmt.Errorf("amount out of range")
	}
	v := w.Int64()
	if neg {
		v = -v
	}
	return v, nil
}

func FormatDOGE(koinu int64) string {
	neg := koinu < 0
	if neg {
		koinu = -koinu
	}
	whole := koinu / KoinuPerDoge
	frac := koinu % KoinuPerDoge
	s := fmt.Sprintf("%d.%08d", whole, frac)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-" {
		s = "0"
	}
	if neg {
		return "-" + s
	}
	return s
}

func ParseKoinu(s string) (int64, error) {
	s = strings.TrimSpace(s)
	n := new(big.Int)
	if _, ok := n.SetString(s, 10); !ok {
		return 0, fmt.Errorf("invalid koinu %q", s)
	}
	if !n.IsInt64() {
		return 0, fmt.Errorf("koinu out of range")
	}
	return n.Int64(), nil
}

// SpendableKoinu sums UTXO values that already have at least minConf confirmations.
func SpendableKoinu(utxos []metalapi.UTXO, minConf int) (int64, error) {
	var sum int64
	for _, u := range utxos {
		if u.Confirmations < minConf {
			continue
		}
		v, err := ParseKoinu(u.Value)
		if err != nil {
			return 0, err
		}
		sum += v
	}
	return sum, nil
}
