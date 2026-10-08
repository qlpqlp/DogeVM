// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/DogecoinVM/dogevm-connect/internal/bridge"
	"github.com/DogecoinVM/dogevm-connect/internal/config"
	"github.com/DogecoinVM/dogevm-connect/internal/dogetx"
	"github.com/DogecoinVM/dogevm-connect/internal/metalapi"
	"github.com/DogecoinVM/dogevm-connect/internal/tracker"
	"github.com/DogecoinVM/dogevm-connect/internal/wallet"
	"github.com/btcsuite/btcd/wire"
)

type Server struct {
	Store   *wallet.Store
	Client  *metalapi.Client
	Tracker *tracker.Tracker
	Static  fs.FS
	Info    *metalapi.Info
	Access  *config.Access
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/auth", s.handleAuth)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/network", s.handleNetwork)
	mux.HandleFunc("/api/wallet", s.handleWallet)
	mux.HandleFunc("/api/wallet/create", s.handleWalletCreate)
	mux.HandleFunc("/api/wallet/import", s.handleWalletImport)
	mux.HandleFunc("/api/wallet/unlock", s.handleWalletUnlock)
	mux.HandleFunc("/api/wallet/lock", s.handleWalletLock)
	mux.HandleFunc("/api/wallet/export", s.handleWalletExport)
	mux.HandleFunc("/api/wallet/remove", s.handleWalletRemove)
	mux.HandleFunc("/api/balances", s.handleBalances)
	mux.HandleFunc("/api/transactions", s.handleTransactions)
	mux.HandleFunc("/api/send", s.handleSend)
	mux.HandleFunc("/api/confirm", s.handleConfirm)
	mux.HandleFunc("/api/bridge/deposit", s.handleBridgeDeposit)
	mux.HandleFunc("/api/bridge/deposit-address", s.handleDepositAddress)
	mux.HandleFunc("/api/bridge/deposits", s.handleDeposits)
	mux.HandleFunc("/api/bridge/withdraw", s.handleWithdraw)
	mux.HandleFunc("/api/bridge/pegout", s.handlePegOut)
	mux.HandleFunc("/api/bridge/activity", s.handleActivity)
	mux.HandleFunc("/api/doge/import", s.handleDogeImport)
	mux.HandleFunc("/api/speed", s.handleSpeed)
	mux.HandleFunc("/api/events", s.handleEventsProxy)

	if s.Static != nil {
		fileServer := http.FileServer(http.FS(s.Static))
		mux.Handle("/", spaHandler(fileServer))
	}
	return withCORS(s.withAuth(mux))
}

func spaHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	msg := err.Error()
	var apiErr *metalapi.APIError
	if errors.As(err, &apiErr) {
		status = apiErr.Status
		msg = apiErr.Msg
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return dec.Decode(dst)
}

func (s *Server) ensureInfo(ctx context.Context) (*metalapi.Info, error) {
	if s.Info != nil {
		return s.Info, nil
	}
	info, err := s.Client.Info(ctx)
	if err != nil {
		return nil, err
	}
	if info.ChainID != wallet.ExpectedChainID || info.DogecoinVMNetwork != wallet.ExpectedDogecoinVMNet {
		return nil, errors.New("unexpected DogecoinVM network - refusing to continue")
	}
	s.Info = info
	return info, nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "dogevm-connect"})
}

func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	info, err := s.ensureInfo(ctx)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	st, err := s.Client.Status(ctx)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"info":   info,
		"status": st,
		"pinned": map[string]string{
			"chainID":           wallet.ExpectedChainID,
			"dogecoinvmNetwork": wallet.ExpectedDogecoinVMNet,
		},
		"docs": "https://metaldoge.com/developers",
	})
}

func (s *Server) handleWallet(w http.ResponseWriter, r *http.Request) {
	addr, created, exists, unlocked, err := s.Store.PublicInfo()
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"exists":    exists,
		"unlocked":  unlocked,
		"address":   addr,
		"createdAt": created,
		"note":      "Same D… address is valid on Dogecoin mainnet and DogecoinVM. Always label the chain before sending.",
	})
}

func (s *Server) handleWalletCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, errors.New("POST only"))
		return
	}
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if err := readJSON(r, &body); err != nil || body.Passphrase == "" {
		writeErr(w, 400, errors.New("passphrase required"))
		return
	}
	sess, err := s.Store.Create(body.Passphrase)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	_ = s.watchDoge(r.Context(), sess.Address)
	writeJSON(w, http.StatusOK, map[string]any{
		"address": sess.Address,
		"warning": "Back up your key (export WIF). If this data is lost, the DOGE is gone. Keys never leave this machine.",
	})
}

func (s *Server) handleWalletImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, errors.New("POST only"))
		return
	}
	var body struct {
		Passphrase string `json:"passphrase"`
		WIF        string `json:"wif"`
		Hex        string `json:"hex"`
	}
	if err := readJSON(r, &body); err != nil || body.Passphrase == "" {
		writeErr(w, 400, errors.New("passphrase required"))
		return
	}
	var sess *wallet.Session
	var err error
	switch {
	case body.WIF != "":
		sess, err = s.Store.ImportWIF(strings.TrimSpace(body.WIF), body.Passphrase)
	case body.Hex != "":
		sess, err = s.Store.ImportHex(strings.TrimSpace(body.Hex), body.Passphrase)
	default:
		writeErr(w, 400, errors.New("provide wif or hex"))
		return
	}
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	_ = s.watchDoge(r.Context(), sess.Address)
	writeJSON(w, http.StatusOK, map[string]any{"address": sess.Address})
}

func (s *Server) handleWalletUnlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, errors.New("POST only"))
		return
	}
	var body struct {
		Passphrase string `json:"passphrase"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	sess, err := s.Store.Unlock(body.Passphrase)
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	_ = s.watchDoge(r.Context(), sess.Address)
	writeJSON(w, http.StatusOK, map[string]any{"address": sess.Address, "unlocked": true})
}

func (s *Server) handleWalletLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, errors.New("POST only"))
		return
	}
	s.Store.Lock()
	writeJSON(w, http.StatusOK, map[string]any{"unlocked": false})
}

func (s *Server) handleWalletExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, errors.New("POST only"))
		return
	}
	wif, err := s.Store.ExportWIF()
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	hx, err := s.Store.ExportHex()
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"wif":     wif,
		"hex":     hx,
		"warning": "Anyone with this key can spend your DOGE on both chains. Store offline.",
	})
}

func (s *Server) handleWalletRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, errors.New("POST only"))
		return
	}
	if err := s.Store.Remove(); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": true})
}

func (s *Server) watchDoge(ctx context.Context, addr string) error {
	if _, err := s.ensureInfo(ctx); err != nil {
		return err
	}
	return s.Client.DogeWatch(ctx, addr)
}

func (s *Server) handleBalances(w http.ResponseWriter, r *http.Request) {
	sess, err := s.Store.Session()
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	ctx := r.Context()
	_ = s.watchDoge(ctx, sess.Address)

	vm, errVM := s.Client.Address(ctx, sess.Address)
	doge, errDoge := s.Client.DogeAddress(ctx, sess.Address)

	out := map[string]any{
		"address": sess.Address,
		"chains": map[string]any{
			"dogecoinvm": chainPayload("DogecoinVM (Metal)", vm, errVM),
			"dogecoin":   chainPayload("Dogecoin mainnet (L1)", doge, errDoge),
		},
		"education": []string{
			"One key → one D… address on both chains. Balances are separate UTXO sets.",
			"Sending on Dogecoin to your D… address does NOT bridge - it stays on L1.",
			"To move onto DogecoinVM, send to your personal deposit address (Bridge tab).",
			"DogecoinVM payments finalize in ~0.2s; Dogecoin needs ~1 minute per confirmation.",
		},
	}
	writeJSON(w, http.StatusOK, out)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Server) handleTransactions(w http.ResponseWriter, r *http.Request) {
	sess, err := s.Store.Session()
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	ctx := r.Context()
	_ = s.watchDoge(ctx, sess.Address)

	type row struct {
		Chain         string `json:"chain"`
		TxID          string `json:"txid"`
		Net           string `json:"net"`
		Confirmations int    `json:"confirmations"`
		Time          int64  `json:"time"`
		ExplorerHint  string `json:"explorerHint"`
	}
	var rows []row

	if vm, err := s.Client.Address(ctx, sess.Address); err == nil {
		for _, h := range vm.History {
			rows = append(rows, row{
				Chain:         "dogecoinvm",
				TxID:          h.TxID,
				Net:           h.Net,
				Confirmations: h.Confirmations,
				Time:          h.Time,
				ExplorerHint:  "https://metaldoge.com/ (DogecoinVM explorer)",
			})
		}
	}
	if doge, err := s.Client.DogeAddress(ctx, sess.Address); err == nil {
		for _, h := range doge.History {
			rows = append(rows, row{
				Chain:         "dogecoin",
				TxID:          h.TxID,
				Net:           h.Net,
				Confirmations: h.Confirmations,
				Time:          h.Time,
				ExplorerHint:  "https://dogechain.info/tx/" + h.TxID,
			})
		}
	}

	// Newest first (best-effort by time).
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].Time > rows[i].Time {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"address":      sess.Address,
		"transactions": rows,
		"transparency": "History comes from metaldoge.com address indexes (DogecoinVM node + Dogecoin watch). Verify important txids on independent explorers.",
	})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, errors.New("POST only"))
		return
	}
	var body struct {
		Chain   string `json:"chain"` // dogecoin | dogecoinvm
		To      string `json:"to"`
		Amount  string `json:"amount"`
		Preview bool   `json:"preview"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	body.Chain = strings.ToLower(strings.TrimSpace(body.Chain))
	if body.Chain != "dogecoin" && body.Chain != "dogecoinvm" {
		writeErr(w, 400, errors.New("chain must be dogecoin or dogecoinvm"))
		return
	}
	sess, err := s.Store.Session()
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	amount, err := dogetx.ParseDOGE(body.Amount)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	ctx := r.Context()
	notes := []string{
		"Signed locally - private key never sent to metaldoge.com.",
		"UTXO values verified from raw transactions before signing.",
	}
	if body.Chain == "dogecoin" {
		notes = append(notes, "This spends Dogecoin mainnet UTXOs only.")
	} else {
		notes = append(notes, "This spends DogecoinVM UTXOs only (final in ~seconds).")
	}
	notes = append(notes, "After broadcast, poll GET /api/confirm?flow=send&chain="+body.Chain+"&txid=... until confirmed is true.")

	built, status, err := s.signPayment(ctx, sess, body.Chain, strings.TrimSpace(body.To), amount, nil, notes)
	if err != nil {
		writeErr(w, status, err)
		return
	}
	if body.Preview {
		writeJSON(w, http.StatusOK, map[string]any{"preview": built.Preview, "txid": built.TxID})
		return
	}
	txid, err := s.Client.Broadcast(ctx, body.Chain, built.Hex)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"txid":    txid,
		"preview": built.Preview,
		"confirm": "/api/confirm?flow=send&chain=" + body.Chain + "&txid=" + txid,
	})
}

func (s *Server) handleBridgeDeposit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, errors.New("POST only"))
		return
	}
	var body struct {
		Amount  string `json:"amount"`
		Preview bool   `json:"preview"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	sess, err := s.Store.Session()
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	amount, err := dogetx.ParseDOGE(body.Amount)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	ctx := r.Context()
	info, err := s.ensureInfo(ctx)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	if minDep, err := dogetx.ParseDOGE(info.MinDeposit); err == nil && amount < minDep {
		writeErr(w, 400, errors.New("amount below minDeposit ("+info.MinDeposit+" DOGE)"))
		return
	}
	if info.MaxDeposit != "" {
		if maxDep, err := dogetx.ParseDOGE(info.MaxDeposit); err == nil && maxDep > 0 && amount > maxDep {
			writeErr(w, 400, errors.New("amount above maxDeposit ("+info.MaxDeposit+" DOGE)"))
			return
		}
	}
	resp, err := s.Client.DepositAddress(ctx, sess.Address)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	if err := bridge.VerifyDepositAddress(info, sess.Address, resp.DepositAddress, resp.RedeemScript); err != nil {
		writeErr(w, 502, err)
		return
	}
	notes := []string{
		"Bridge deposit: spends Dogecoin L1 UTXOs to your personal deposit address.",
		"After enough Dogecoin confirmations, DogecoinVM is credited minus vmFee (" + info.VMFee + " DOGE).",
		"Poll GET /api/confirm?flow=deposit&txid=... until available is true.",
	}
	built, status, err := s.signPayment(ctx, sess, "dogecoin", resp.DepositAddress, amount, nil, notes)
	if err != nil {
		writeErr(w, status, err)
		return
	}
	if body.Preview {
		writeJSON(w, http.StatusOK, map[string]any{
			"preview":        built.Preview,
			"txid":           built.TxID,
			"depositAddress": resp.DepositAddress,
			"creditTo":       resp.CreditTo,
		})
		return
	}
	txid, err := s.Client.Broadcast(ctx, "dogecoin", built.Hex)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"txid":           txid,
		"preview":        built.Preview,
		"depositAddress": resp.DepositAddress,
		"creditTo":       resp.CreditTo,
		"confirm":        "/api/confirm?flow=deposit&txid=" + txid,
	})
}

func (s *Server) handleDepositAddress(w http.ResponseWriter, r *http.Request) {
	sess, err := s.Store.Session()
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	ctx := r.Context()
	info, err := s.ensureInfo(ctx)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	resp, err := s.Client.DepositAddress(ctx, sess.Address)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	if err := bridge.VerifyDepositAddress(info, sess.Address, resp.DepositAddress, resp.RedeemScript); err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"depositAddress": resp.DepositAddress,
		"creditTo":       resp.CreditTo,
		"verified":       true,
		"chain":          "dogecoin",
		"education": []string{
			"Send DOGE on Dogecoin mainnet to this deposit address only.",
			"After enough Dogecoin confirmations, the bridge credits your DogecoinVM balance (minus vmFee).",
			"DOGE sent to your normal D… wallet address stays on Dogecoin - it does not bridge.",
			"Deposit address redeem script was rebuilt locally and matched the API response.",
		},
		"limits": map[string]string{
			"minDeposit": info.MinDeposit,
			"maxDeposit": info.MaxDeposit,
			"vmFee":      info.VMFee,
		},
	})
}

func (s *Server) handleDeposits(w http.ResponseWriter, r *http.Request) {
	sess, err := s.Store.Session()
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	list, err := s.Client.Deposits(r.Context(), sess.Address)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deposits": list})
}

func (s *Server) handleWithdraw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, errors.New("POST only"))
		return
	}
	var body struct {
		To      string `json:"to"`
		Amount  string `json:"amount"`
		Preview bool   `json:"preview"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, 400, err)
		return
	}
	sess, err := s.Store.Session()
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	amount, err := dogetx.ParseDOGE(body.Amount)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	ctx := r.Context()
	info, err := s.ensureInfo(ctx)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	minOut, err := dogetx.ParseDOGE(info.MinPegOut)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	if amount < minOut {
		writeErr(w, 400, errors.New("amount below minPegOut ("+info.MinPegOut+" DOGE)"))
		return
	}
	if strings.TrimSpace(body.To) == info.PegAddress {
		writeErr(w, 400, errors.New("cannot withdraw to the bridge peg address"))
		return
	}

	tag, err := bridge.DVMOTag(strings.TrimSpace(body.To))
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	decoded, err := bridge.DecodeDVMOTag(tag)
	if err != nil || decoded != strings.TrimSpace(body.To) {
		writeErr(w, 400, errors.New("DVMO tag round-trip failed"))
		return
	}
	opRet, err := bridge.OPReturnScript(tag)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	extra := wire.NewTxOut(0, opRet)

	notes := []string{
		"Withdrawal: pay DogecoinVM reserve + DVMO tag naming Dogecoin payout address.",
		"Decoded DVMO destination (verify before broadcast): " + decoded,
		"Bridge pays about amount - dogeFee (" + info.DogeFee + " DOGE) on Dogecoin once the VM tx is final.",
		"Poll GET /api/confirm?flow=withdraw&txid=... until available is true.",
	}
	built, status, err := s.signPayment(ctx, sess, "dogecoinvm", info.ReserveAddress, amount, extra, notes)
	if err != nil {
		writeErr(w, status, err)
		return
	}
	built.Preview.PegOutTo = decoded
	built.Preview.BridgeFeeDOGE = info.DogeFee

	if body.Preview {
		writeJSON(w, http.StatusOK, map[string]any{"preview": built.Preview, "txid": built.TxID})
		return
	}
	txid, err := s.Client.Broadcast(ctx, "dogecoinvm", built.Hex)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"txid":    txid,
		"preview": built.Preview,
		"track":   "/api/bridge/pegout?txid=" + txid,
		"confirm": "/api/confirm?flow=withdraw&txid=" + txid,
	})
}

func (s *Server) handlePegOut(w http.ResponseWriter, r *http.Request) {
	txid := r.URL.Query().Get("txid")
	if txid == "" {
		writeErr(w, 400, errors.New("txid required"))
		return
	}
	out, err := s.Client.PegOut(r.Context(), txid)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	list, err := s.Client.Activity(r.Context())
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"activity": list})
}

func (s *Server) handleDogeImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, errors.New("POST only"))
		return
	}
	sess, err := s.Store.Session()
	if err != nil {
		writeErr(w, 401, err)
		return
	}
	var body struct {
		TxID string `json:"txid"`
	}
	if err := readJSON(r, &body); err != nil || body.TxID == "" {
		writeErr(w, 400, errors.New("txid required"))
		return
	}
	if err := s.Client.DogeImport(r.Context(), sess.Address, body.TxID); err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": true})
}

func (s *Server) handleSpeed(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Tracker.Speed())
}

func (s *Server) handleEventsProxy(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, errors.New("streaming unsupported"))
		return
	}
	ctx := r.Context()
	body, err := s.Client.OpenEvents(ctx)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	buf := make([]byte, 4096)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func ListenAndServe(addr string, srv *Server) error {
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("DogeVM Connect listening on http://%s", addr)
	return httpSrv.ListenAndServe()
}
