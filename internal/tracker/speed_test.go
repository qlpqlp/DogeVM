// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package tracker

import "testing"

func TestSpeedFromDurationAlignment(t *testing.T) {
	dogeG, dogeM := speedFromDuration(60_000)
	vmG, vmM := speedFromDuration(200) // ~300×

	if dogeM < 0.9 || dogeM > 1.1 {
		t.Fatalf("doge multiplier want ~1, got %v", dogeM)
	}
	if vmM < 250 || vmM > 350 {
		t.Fatalf("vm multiplier want ~300, got %v", vmM)
	}
	if dogeG > 25 {
		t.Fatalf("mainnet gauge should sit low, got %v", dogeG)
	}
	if vmG < 85 {
		t.Fatalf("VM gauge should sit high (~300×), got %v", vmG)
	}
	if vmG <= dogeG {
		t.Fatalf("VM (%v) must score above Dogecoin (%v)", vmG, dogeG)
	}
}
