// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package server

import (
	"context"

	"github.com/DogecoinVM/dogevm-connect/internal/dogetx"
	"github.com/DogecoinVM/dogevm-connect/internal/metalapi"
	"github.com/DogecoinVM/dogevm-connect/internal/wallet"
	"github.com/btcsuite/btcd/wire"
)

func (s *Server) signPayment(ctx context.Context, sess *wallet.Session, chain, to string, amount int64, extra *wire.TxOut, notes []string) (*dogetx.BuildResult, int, error) {
	if _, err := s.ensureInfo(ctx); err != nil {
		return nil, 502, err
	}
	var view *metalapi.AddressView
	var err error
	if chain == "dogecoinvm" {
		view, err = s.Client.Address(ctx, sess.Address)
	} else {
		_ = s.watchDoge(ctx, sess.Address)
		view, err = s.Client.DogeAddress(ctx, sess.Address)
	}
	if err != nil {
		return nil, 502, err
	}
	utxos, err := dogetx.UTXOsFromView(view)
	if err != nil {
		return nil, 500, err
	}
	utxos, err = dogetx.EnrichVerified(func(txid string) (string, error) {
		return s.Client.RawTx(ctx, chain, txid)
	}, utxos)
	if err != nil {
		return nil, 502, err
	}
	built, err := dogetx.BuildPayment(sess.PrivateKey, sess.Address, utxos, to, amount, extra, notes, chain)
	if err != nil {
		return nil, 400, err
	}
	return built, 200, nil
}
