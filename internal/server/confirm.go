// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/DogecoinVM/dogevm-connect/internal/dogetx"
	"github.com/DogecoinVM/dogevm-connect/internal/metalapi"
	"github.com/DogecoinVM/dogevm-connect/internal/wallet"
)

type balanceSnap struct {
	Chain     string `json:"chain"`
	Confirmed string `json:"confirmed"`
	Pending   string `json:"pending"`
	Spendable string `json:"spendable"`
	Unit      string `json:"unit"`
}

func (s *Server) walletAddr() (string, error) {
	addr, _, exists, _, err := s.Store.PublicInfo()
	if err != nil {
		return "", err
	}
	if !exists || addr == "" {
		return "", wallet.ErrNoWallet
	}
	return addr, nil
}

func (s *Server) chainView(ctx context.Context, chain, addr string) (*metalapi.AddressView, error) {
	if chain == "dogecoinvm" {
		return s.Client.Address(ctx, addr)
	}
	_ = s.watchDoge(ctx, addr)
	return s.Client.DogeAddress(ctx, addr)
}

func snapBalance(chain string, view *metalapi.AddressView) balanceSnap {
	snap := balanceSnap{Chain: chain, Confirmed: "0", Pending: "0", Spendable: "0", Unit: "DOGE"}
	if view == nil {
		return snap
	}
	if view.Confirmed != "" {
		snap.Confirmed = view.Confirmed
	}
	if view.Pending != "" {
		snap.Pending = view.Pending
	}
	if k, err := dogetx.SpendableKoinu(view.UTXOs, 1); err == nil {
		snap.Spendable = dogetx.FormatDOGE(k)
	}
	return snap
}

func spendablePositive(snap balanceSnap) bool {
	k, err := dogetx.ParseDOGE(snap.Spendable)
	return err == nil && k > 0
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, errors.New("GET only"))
		return
	}
	flow := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("flow")))
	if flow == "" {
		flow = "send"
	}
	txid := strings.TrimSpace(r.URL.Query().Get("txid"))
	if txid == "" {
		writeErr(w, http.StatusBadRequest, errors.New("txid required"))
		return
	}
	addr, err := s.walletAddr()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	switch flow {
	case "send":
		s.confirmSend(w, r, addr, txid)
	case "deposit":
		s.confirmDeposit(w, r, addr, txid)
	case "withdraw":
		s.confirmWithdraw(w, r, addr, txid)
	default:
		writeErr(w, http.StatusBadRequest, errors.New("flow must be send, deposit, or withdraw"))
	}
}

func (s *Server) confirmSend(w http.ResponseWriter, r *http.Request, addr, txid string) {
	chain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("chain")))
	if chain != "dogecoin" && chain != "dogecoinvm" {
		writeErr(w, http.StatusBadRequest, errors.New("chain must be dogecoin or dogecoinvm"))
		return
	}
	ctx := r.Context()
	view, err := s.chainView(ctx, chain, addr)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	found, confs := historyConf(view, txid)
	if !found {
		var tv *metalapi.TxView
		var txErr error
		if chain == "dogecoin" {
			tv, txErr = s.Client.DogeTx(ctx, txid)
		} else {
			tv, txErr = s.Client.Tx(ctx, txid)
		}
		if txErr == nil && tv != nil {
			found = true
			confs = tv.Confirmations
			if tv.TxID != "" {
				txid = tv.TxID
			}
		}
	}
	const required = 1
	confirmed := found && confs >= required
	snap := snapBalance(chain, view)
	writeJSON(w, http.StatusOK, map[string]any{
		"flow":          "send",
		"chain":         chain,
		"txid":          txid,
		"address":       addr,
		"found":         found,
		"confirmations": confs,
		"required":      required,
		"confirmed":     confirmed,
		"balance":       snap,
		"available":     confirmed && spendablePositive(snap),
		"detail":        "confirmed is true once this tx has at least 1 confirmation. available is true when it is confirmed and this wallet still has a spendable balance on the same chain.",
	})
}

func (s *Server) confirmDeposit(w http.ResponseWriter, r *http.Request, addr, txid string) {
	ctx := r.Context()
	list, err := s.Client.Deposits(ctx, addr)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	var dep *metalapi.Deposit
	for i := range list {
		if strings.EqualFold(list[i].TxID, txid) {
			dep = &list[i]
			break
		}
	}
	vm, err := s.chainView(ctx, "dogecoinvm", addr)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	snap := snapBalance("dogecoinvm", vm)
	required := 1
	if info, infoErr := s.ensureInfo(ctx); infoErr == nil && info.DepositConfirmations > 0 {
		required = info.DepositConfirmations
	}
	confs := 0
	status := ""
	credit := ""
	if dep != nil {
		confs = dep.Confirmations
		status = dep.Status
		credit = dep.CreditTxID
		if dep.Required > 0 {
			required = dep.Required
		}
	}
	credited := dep != nil && (credit != "" || strings.EqualFold(status, "credited"))
	confirmed := dep != nil && (credited || confs >= required)
	writeJSON(w, http.StatusOK, map[string]any{
		"flow":          "deposit",
		"chain":         "dogecoin",
		"creditChain":   "dogecoinvm",
		"txid":          txid,
		"address":       addr,
		"found":         dep != nil,
		"status":        status,
		"confirmations": confs,
		"required":      required,
		"creditTxid":    credit,
		"confirmed":     confirmed,
		"balance":       snap,
		"available":     credited && spendablePositive(snap),
		"detail":        "Poll with the Dogecoin deposit txid. confirmed follows Dogecoin confirmations or a bridge credit. available is true once the bridge has credited DogecoinVM and that wallet has a spendable balance.",
	})
}

func (s *Server) confirmWithdraw(w http.ResponseWriter, r *http.Request, addr, txid string) {
	ctx := r.Context()
	peg, err := s.Client.PegOut(ctx, txid)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	doge, viewErr := s.chainView(ctx, "dogecoin", addr)
	if viewErr != nil {
		writeErr(w, 502, viewErr)
		return
	}
	snap := snapBalance("dogecoin", doge)
	payConfs := 0
	payTx := ""
	status := ""
	to := ""
	if peg != nil {
		payConfs = peg.PaymentConfirmations
		payTx = peg.PaymentTxID
		status = peg.Status
		to = peg.To
	}
	if payTx != "" && payConfs < 1 {
		if tv, txErr := s.Client.DogeTx(ctx, payTx); txErr == nil && tv != nil && tv.Confirmations > payConfs {
			payConfs = tv.Confirmations
		}
	}
	payoutConfirmed := payTx != "" && payConfs >= 1
	writeJSON(w, http.StatusOK, map[string]any{
		"flow":                 "withdraw",
		"chain":                "dogecoinvm",
		"payoutChain":          "dogecoin",
		"txid":                 txid,
		"address":              addr,
		"found":                peg != nil && (status != "" || payTx != ""),
		"status":               status,
		"paymentTxid":          payTx,
		"paymentConfirmations": payConfs,
		"payoutTo":             to,
		"required":             1,
		"confirmed":            payoutConfirmed,
		"balance":              snap,
		"available":            payoutConfirmed,
		"detail":               "Poll with the DogecoinVM withdraw txid. confirmed and available are true once the Dogecoin payout has at least 1 confirmation. balance is this wallet on Dogecoin L1, which changes only if the payout address is this wallet.",
	})
}

func historyConf(view *metalapi.AddressView, txid string) (bool, int) {
	if view == nil {
		return false, 0
	}
	for _, h := range view.History {
		if strings.EqualFold(h.TxID, txid) {
			return true, h.Confirmations
		}
	}
	return false, 0
}

func chainPayload(label string, view *metalapi.AddressView, err error) map[string]any {
	payload := map[string]any{
		"label": label,
		"error": errString(err),
		"data":  view,
	}
	if err == nil && view != nil {
		payload["confirmed"] = view.Confirmed
		payload["pending"] = view.Pending
		if k, e := dogetx.SpendableKoinu(view.UTXOs, 1); e == nil {
			payload["spendable"] = dogetx.FormatDOGE(k)
		}
	}
	return payload
}
