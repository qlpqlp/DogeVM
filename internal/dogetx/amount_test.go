// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package dogetx

import (
	"testing"

	"github.com/DogecoinVM/dogevm-connect/internal/metalapi"
)

func TestParseFormatDOGE(t *testing.T) {
	v, err := ParseDOGE("1.5")
	if err != nil {
		t.Fatal(err)
	}
	if v != 150_000_000 {
		t.Fatalf("got %d", v)
	}
	if FormatDOGE(v) != "1.5" {
		t.Fatalf("format %q", FormatDOGE(v))
	}
}

func TestSpendableKoinu(t *testing.T) {
	sum, err := SpendableKoinu([]metalapi.UTXO{
		{Value: "100000000", Confirmations: 1},
		{Value: "50000000", Confirmations: 0},
		{Value: "25000000", Confirmations: 3},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if FormatDOGE(sum) != "1.25" {
		t.Fatalf("got %s", FormatDOGE(sum))
	}
}
