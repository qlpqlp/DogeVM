// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package bridge

import "testing"

func TestDVMORoundTrip(t *testing.T) {
	// Deterministic: hash160 of zeros encoded as P2PKH.
	addr := "D8du9rHM3X8VSyGNUqYUsVAwWp8XUwYPZr" // may fail if invalid - use generated
	// Use a known valid Dogecoin P2PKH from pubkeyhash zeros is invalid checksum.
	// Round-trip via AddressHash160 requires a real address - create via wallet in integration.
	_ = addr
	tag := make([]byte, 25)
	copy(tag[0:4], []byte("DVMO"))
	tag[4] = 0x00
	for i := 5; i < 25; i++ {
		tag[i] = byte(i)
	}
	decoded, err := DecodeDVMOTag(tag)
	if err != nil {
		t.Fatal(err)
	}
	again, err := DVMOTag(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(tag) {
		t.Fatalf("mismatch\n%x\n%x", again, tag)
	}
}
