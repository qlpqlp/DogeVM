// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package bridge

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/DogecoinVM/dogevm-connect/internal/metalapi"
	"github.com/DogecoinVM/dogevm-connect/internal/wallet"
)

// BuildRedeemScript rebuilds the personal deposit redeem script from /api/info
// and the user's DogecoinVM address, per the DogecoinVM API docs.
func BuildRedeemScript(info *metalapi.Info, creditAddress string) ([]byte, error) {
	h160, addrType, err := wallet.AddressHash160(creditAddress)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, 21)
	payload[0] = addrType
	copy(payload[1:], h160)

	builder := txscript.NewScriptBuilder()
	builder.AddData(payload)
	builder.AddOp(txscript.OP_DROP)

	if info.Signers.Required < 1 || len(info.Signers.PublicKeys) < info.Signers.Required {
		return nil, errors.New("invalid signer set from /api/info")
	}
	builder.AddInt64(int64(info.Signers.Required))
	for _, pkHex := range info.Signers.PublicKeys {
		pk, err := hex.DecodeString(pkHex)
		if err != nil {
			return nil, fmt.Errorf("signer pubkey: %w", err)
		}
		builder.AddData(pk)
	}
	builder.AddInt64(int64(len(info.Signers.PublicKeys)))
	builder.AddOp(txscript.OP_CHECKMULTISIG)
	return builder.Script()
}

// VerifyDepositAddress rebuilds the expected P2SH and compares to the API response.
func VerifyDepositAddress(info *metalapi.Info, creditAddress, depositAddress, redeemHex string) error {
	script, err := BuildRedeemScript(info, creditAddress)
	if err != nil {
		return err
	}
	if redeemHex != "" {
		got, err := hex.DecodeString(redeemHex)
		if err != nil {
			return fmt.Errorf("redeemScript hex: %w", err)
		}
		if !bytes.Equal(got, script) {
			return errors.New("redeemScript from API does not match rebuilt script - refusing deposit address")
		}
	}
	addr, err := btcutil.NewAddressScriptHash(script, &wallet.DogeMainNetParams)
	if err != nil {
		return err
	}
	if addr.EncodeAddress() != depositAddress {
		return fmt.Errorf("deposit address mismatch: expected %s, API gave %s", addr.EncodeAddress(), depositAddress)
	}
	return nil
}

// DVMOTag builds the 25-byte OP_RETURN payload naming a Dogecoin payout address.
func DVMOTag(dogeAddress string) ([]byte, error) {
	h160, addrType, err := wallet.AddressHash160(dogeAddress)
	if err != nil {
		return nil, err
	}
	tag := make([]byte, 25)
	copy(tag[0:4], []byte("DVMO"))
	tag[4] = addrType
	copy(tag[5:], h160)
	return tag, nil
}

// DecodeDVMOTag parses a DVMO tag back to a Dogecoin address (for pre-sign review).
func DecodeDVMOTag(tag []byte) (string, error) {
	if len(tag) != 25 || string(tag[0:4]) != "DVMO" {
		return "", errors.New("not a DVMO tag")
	}
	h160 := tag[5:25]
	switch tag[4] {
	case 0x00:
		a, err := btcutil.NewAddressPubKeyHash(h160, &wallet.DogeMainNetParams)
		if err != nil {
			return "", err
		}
		return a.EncodeAddress(), nil
	case 0x01:
		a, err := btcutil.NewAddressScriptHashFromHash(h160, &wallet.DogeMainNetParams)
		if err != nil {
			return "", err
		}
		return a.EncodeAddress(), nil
	default:
		return "", errors.New("unknown DVMO address type")
	}
}

// OPReturnScript wraps a data push in OP_RETURN.
func OPReturnScript(data []byte) ([]byte, error) {
	return txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).AddData(data).Script()
}
