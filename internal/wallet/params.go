// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package wallet

import "github.com/btcsuite/btcd/chaincfg"

// DogeMainNetParams are Dogecoin mainnet address/WIF parameters.
// DogecoinVM uses the same version bytes - one key, one D… address on both chains.
var DogeMainNetParams = chaincfg.Params{
	Name:             "dogecoin-mainnet",
	Net:              0xc0c0c0c0,
	PubKeyHashAddrID: 0x1e, // 30 → D…
	ScriptHashAddrID: 0x16, // 22 → 9… / A…
	PrivateKeyID:     0x9e, // 158
	HDCoinType:       3,
}
