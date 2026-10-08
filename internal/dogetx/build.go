// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package dogetx

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/DogecoinVM/dogevm-connect/internal/metalapi"
	"github.com/DogecoinVM/dogevm-connect/internal/wallet"
)

// Dogecoin Core 1.14 / DogecoinVM relay rules (from metaldoge docs).
const (
	MinRelayFeePerKB   int64 = 100_000 // 0.001 DOGE / kB = 100 koinu / byte
	DustSoftLimit      int64 = 1_000_000  // 0.01 DOGE - soft dust adds to required fee
	DustHardLimit      int64 = 100_000    // 0.001 DOGE - non-standard
	EstInputBytes      int   = 148
	EstOutputBytes     int   = 34
	EstOverheadBytes   int   = 10
)

type Spendable struct {
	TxID          string
	Vout          uint32
	Value         int64
	PkScript      []byte
	Confirmations int
}

type BuildResult struct {
	Tx           *wire.MsgTx
	Hex          string
	TxID         string
	FeeKoinu     int64
	ChangeKoinu  int64
	SelectedIn   []Spendable
	SizeBytes    int
	Preview      PaymentPreview
}

type PaymentPreview struct {
	Chain         string   `json:"chain"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	AmountDOGE    string   `json:"amountDoge"`
	FeeDOGE       string   `json:"feeDoge"`
	ChangeDOGE    string   `json:"changeDoge"`
	Inputs        int      `json:"inputs"`
	SizeBytes     int      `json:"sizeBytes"`
	Notes         []string `json:"notes,omitempty"`
	PegOutTo      string   `json:"pegOutTo,omitempty"`
	BridgeFeeDOGE string   `json:"bridgeFeeDoge,omitempty"`
}

// VerifyRawUTXO fetches raw hex, checks it hashes to txid, and returns the output value/script.
func VerifyRawUTXO(rawHex, txid string, vout uint32) (value int64, pkScript []byte, err error) {
	raw, err := hex.DecodeString(rawHex)
	if err != nil {
		return 0, nil, err
	}
	tx, err := btcutil.NewTxFromBytes(raw)
	if err != nil {
		return 0, nil, err
	}
	msg := tx.MsgTx()
	h := msg.TxHash()
	if h.String() != txid {
		return 0, nil, fmt.Errorf("raw tx hash %s != claimed txid %s", h.String(), txid)
	}
	if int(vout) >= len(msg.TxOut) {
		return 0, nil, fmt.Errorf("vout %d out of range", vout)
	}
	out := msg.TxOut[vout]
	return out.Value, append([]byte(nil), out.PkScript...), nil
}

func UTXOsFromView(view *metalapi.AddressView) ([]Spendable, error) {
	out := make([]Spendable, 0, len(view.UTXOs))
	for _, u := range view.UTXOs {
		v, err := ParseKoinu(u.Value)
		if err != nil {
			return nil, err
		}
		script, err := hex.DecodeString(u.Script)
		if err != nil {
			return nil, err
		}
		out = append(out, Spendable{
			TxID:          u.TxID,
			Vout:          u.Vout,
			Value:         v,
			PkScript:      script,
			Confirmations: u.Confirmations,
		})
	}
	return out, nil
}

func estimateSize(nIn, nOut int) int {
	return EstOverheadBytes + nIn*EstInputBytes + nOut*EstOutputBytes
}

func requiredFee(size int, outputs []int64) int64 {
	fee := int64(size) * MinRelayFeePerKB / 1000
	if fee < MinRelayFeePerKB/1000 {
		fee = 100 // at least 100 koinu
	}
	// Round up to per-byte minimum.
	minBySize := int64(size) * 100
	if fee < minBySize {
		fee = minBySize
	}
	for _, o := range outputs {
		if o > 0 && o < DustSoftLimit {
			fee += DustSoftLimit
		}
	}
	return fee
}

func selectCoins(utxos []Spendable, need int64) ([]Spendable, int64, error) {
	// Largest-first greedy selection.
	sorted := append([]Spendable(nil), utxos...)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Value > sorted[i].Value {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	var selected []Spendable
	var sum int64
	for _, u := range sorted {
		selected = append(selected, u)
		sum += u.Value
		if sum >= need {
			return selected, sum, nil
		}
	}
	return nil, sum, fmt.Errorf("insufficient funds: have %s DOGE, need about %s DOGE", FormatDOGE(sum), FormatDOGE(need))
}

// BuildPayment creates a signed P2PKH→P2PKH/P2SH payment with change.
func BuildPayment(priv *btcec.PrivateKey, fromAddr string, utxos []Spendable, toAddr string, amount int64, extraOut *wire.TxOut, notes []string, chain string) (*BuildResult, error) {
	if amount <= 0 {
		return nil, errors.New("amount must be positive")
	}
	if amount < DustHardLimit {
		return nil, fmt.Errorf("amount below hard dust limit (%s DOGE)", FormatDOGE(DustHardLimit))
	}
	toScript, err := payToAddrScript(toAddr)
	if err != nil {
		return nil, err
	}
	changeScript, err := payToAddrScript(fromAddr)
	if err != nil {
		return nil, err
	}

	// Iterate selection with fee that depends on size / change.
	var (
		selected []Spendable
		sum      int64
		fee      int64
		change   int64
		nOut     int
	)
	for attempt := 0; attempt < 8; attempt++ {
		nOut = 1 // payment
		if extraOut != nil {
			nOut++
		}
		// Assume change first; drop later if dust.
		withChangeOuts := nOut + 1
		size := estimateSize(max(1, len(selected)), withChangeOuts)
		outs := []int64{amount}
		if extraOut != nil {
			outs = append(outs, 0)
		}
		outs = append(outs, DustSoftLimit) // assume change exists for fee calc
		fee = requiredFee(size, outs)
		need := amount + fee
		selected, sum, err = selectCoins(utxos, need)
		if err != nil {
			return nil, err
		}
		size = estimateSize(len(selected), withChangeOuts)
		fee = requiredFee(size, outs)
		change = sum - amount - fee
		if change < 0 {
			continue
		}
		if change > 0 && change < DustHardLimit {
			// Fold dust change into fee.
			fee += change
			change = 0
		}
		break
	}
	if change < 0 {
		return nil, errors.New("could not fund transaction after fee")
	}

	tx := wire.NewMsgTx(1)
	for _, in := range selected {
		hash, err := chainhash.NewHashFromStr(in.TxID)
		if err != nil {
			return nil, err
		}
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, in.Vout), nil, nil))
	}
	tx.AddTxOut(wire.NewTxOut(amount, toScript))
	if extraOut != nil {
		tx.AddTxOut(extraOut)
	}
	if change > 0 {
		tx.AddTxOut(wire.NewTxOut(change, changeScript))
	}

	if err := signP2PKH(tx, selected, priv); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return nil, err
	}
	size := buf.Len()
	// Recompute fee from actual size for preview accuracy.
	actualFee := sum - amount - change
	if extraOut != nil {
		// extraOut value is 0 for OP_RETURN
	}

	preview := PaymentPreview{
		Chain:      chain,
		From:       fromAddr,
		To:         toAddr,
		AmountDOGE: FormatDOGE(amount),
		FeeDOGE:    FormatDOGE(actualFee),
		ChangeDOGE: FormatDOGE(change),
		Inputs:     len(selected),
		SizeBytes:  size,
		Notes:      notes,
	}

	return &BuildResult{
		Tx:          tx,
		Hex:         hex.EncodeToString(buf.Bytes()),
		TxID:        tx.TxHash().String(),
		FeeKoinu:    actualFee,
		ChangeKoinu: change,
		SelectedIn:  selected,
		SizeBytes:   size,
		Preview:     preview,
	}, nil
}

func payToAddrScript(addr string) ([]byte, error) {
	a, err := btcutil.DecodeAddress(addr, &wallet.DogeMainNetParams)
	if err != nil {
		return nil, err
	}
	return txscript.PayToAddrScript(a)
}

func signP2PKH(tx *wire.MsgTx, inputs []Spendable, priv *btcec.PrivateKey) error {
	pub := priv.PubKey().SerializeCompressed()
	for i, in := range inputs {
		sigHash, err := txscript.CalcSignatureHash(in.PkScript, txscript.SigHashAll, tx, i)
		if err != nil {
			return err
		}
		sig := ecdsa.Sign(priv, sigHash)
		sigBytes := append(sig.Serialize(), byte(txscript.SigHashAll))
		builder := txscript.NewScriptBuilder()
		builder.AddData(sigBytes)
		builder.AddData(pub)
		script, err := builder.Script()
		if err != nil {
			return err
		}
		tx.TxIn[i].SignatureScript = script
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// EnrichVerified replaces API-reported UTXO values/scripts with verified raw-tx data.
func EnrichVerified(fetch func(txid string) (string, error), utxos []Spendable) ([]Spendable, error) {
	out := make([]Spendable, len(utxos))
	for i, u := range utxos {
		raw, err := fetch(u.TxID)
		if err != nil {
			return nil, fmt.Errorf("rawtx %s: %w", u.TxID, err)
		}
		val, script, err := VerifyRawUTXO(raw, u.TxID, u.Vout)
		if err != nil {
			return nil, err
		}
		u.Value = val
		u.PkScript = script
		out[i] = u
	}
	return out, nil
}
