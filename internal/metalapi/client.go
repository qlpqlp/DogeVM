// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package metalapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "https://metaldoge.com"

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

type APIError struct {
	Status int
	Msg    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("metaldoge api %d: %s", e.Status, e.Msg)
}

type Info struct {
	ChainID              string `json:"chainID"`
	DogecoinVMNetwork    string `json:"dogecoinvmNetwork"`
	DogecoinNetwork      string `json:"dogecoinNetwork"`
	PegAddress           string `json:"pegAddress"`
	ReserveAddress       string `json:"reserveAddress"`
	DepositConfirmations int    `json:"depositConfirmations"`
	ConfirmationTiers    []struct {
		UpTo          string `json:"upTo"`
		Confirmations int    `json:"confirmations"`
	} `json:"confirmationTiers"`
	VMFee          string `json:"vmFee"`
	DogeFee        string `json:"dogeFee"`
	MinDeposit     string `json:"minDeposit"`
	MaxDeposit     string `json:"maxDeposit"`
	MinPegOut      string `json:"minPegOut"`
	MaxCirculating string `json:"maxCirculating"`
	DogeWallet     bool   `json:"dogeWallet"`
	Signers        struct {
		Required   int      `json:"required"`
		PublicKeys []string `json:"publicKeys"`
	} `json:"signers"`
	DogecoinVersions struct {
		P2PKH int `json:"p2pkh"`
		P2SH  int `json:"p2sh"`
		WIF   int `json:"wif"`
	} `json:"dogecoinVersions"`
}

type Finality struct {
	Payments int `json:"payments"`
	MedianMs int `json:"medianMs"`
	P90Ms    int `json:"p90Ms"`
	LatestMs int `json:"latestMs"`
}

type Audit struct {
	Solvent               bool   `json:"solvent"`
	Locked                string `json:"locked"`
	Circulating           string `json:"circulating"`
	PendingPegIns         string `json:"pendingPegIns"`
	PendingPegOuts        string `json:"pendingPegOuts"`
	Surplus               string `json:"surplus"`
	UnclaimedOnDogecoin   string `json:"unclaimedOnDogecoin"`
	UnclaimedOnDogecoinVM string `json:"unclaimedOnDogecoinVM"`
}

type Status struct {
	DogecoinVMHeight int      `json:"dogecoinvmHeight"`
	DogecoinHeight   int      `json:"dogecoinHeight"`
	Finality         Finality `json:"finality"`
	Audit            *Audit   `json:"audit"`
	Updated          string   `json:"updated"`
	Paused           any      `json:"paused,omitempty"`
	Error            string   `json:"error,omitempty"`
}

type UTXO struct {
	TxID          string `json:"txid"`
	Vout          uint32 `json:"vout"`
	Value         string `json:"value"` // koinu
	Script        string `json:"script"`
	Confirmations int    `json:"confirmations"`
}

type HistoryItem struct {
	TxID          string `json:"txid"`
	Confirmations int    `json:"confirmations"`
	Net           string `json:"net"`
	Time          int64  `json:"time"`
}

type AddressView struct {
	Address     string        `json:"address"`
	Confirmed   string        `json:"confirmed"`
	Pending     string        `json:"pending"`
	UTXOs       []UTXO        `json:"utxos"`
	History     []HistoryItem `json:"history"`
	WatchedFrom int           `json:"watchedFrom,omitempty"`
	IndexedTo   int           `json:"indexedTo,omitempty"`
}

type TxView struct {
	TxID          string `json:"txid"`
	Confirmations int    `json:"confirmations"`
	BlockHash     string `json:"blockHash,omitempty"`
	Time          int64  `json:"time"`
	Kind          string `json:"kind,omitempty"`
	Label         string `json:"label,omitempty"`
	Fee           string `json:"fee,omitempty"`
	DogecoinTxID  string `json:"dogecoinTxid,omitempty"`
	Inputs        []struct {
		Address string `json:"address"`
		Value   string `json:"value"`
		Note    string `json:"note,omitempty"`
	} `json:"inputs"`
	Outputs []struct {
		Address string `json:"address"`
		Value   string `json:"value"`
		Note    string `json:"note,omitempty"`
	} `json:"outputs"`
}

type DepositAddressResp struct {
	DepositAddress string `json:"depositAddress"`
	CreditTo       string `json:"creditTo"`
	RedeemScript   string `json:"redeemScript"`
}

type Deposit struct {
	TxID          string `json:"txid"`
	Vout          int    `json:"vout"`
	Amount        string `json:"amount"`
	Confirmations int    `json:"confirmations"`
	Required      int    `json:"required"`
	Status        string `json:"status"`
	CreditTxID    string `json:"creditTxid,omitempty"`
	Credited      string `json:"credited,omitempty"`
	Reason        string `json:"reason,omitempty"`
	RefundTxID    string `json:"refundTxid,omitempty"`
}

type PegOut struct {
	Status               string `json:"status"`
	Amount               string `json:"amount"`
	Pays                 string `json:"pays"`
	To                   string `json:"to"`
	PaymentTxID          string `json:"paymentTxid,omitempty"`
	PaymentConfirmations int    `json:"paymentConfirmations,omitempty"`
}

type ActivityItem struct {
	Type           string `json:"type"`
	Status         string `json:"status"`
	Amount         string `json:"amount"`
	Pays           string `json:"pays,omitempty"`
	To             string `json:"to,omitempty"`
	DogecoinVMTxID string `json:"dogecoinvmTxid,omitempty"`
	DogecoinTxID   string `json:"dogecoinTxid,omitempty"`
	Time           int64  `json:"time"`
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "dogevm-connect/1.0")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var er struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &er)
		msg := er.Error
		if msg == "" {
			msg = string(body)
		}
		return &APIError{Status: resp.StatusCode, Msg: msg}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	var buf bytes.Buffer
	if in != nil {
		if err := json.NewEncoder(&buf).Encode(in); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "dogevm-connect/1.0")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var er struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &er)
		msg := er.Error
		if msg == "" {
			msg = string(body)
		}
		return &APIError{Status: resp.StatusCode, Msg: msg}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

func (c *Client) Info(ctx context.Context) (*Info, error) {
	var info Info
	if err := c.get(ctx, "/api/info", &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) Status(ctx context.Context) (*Status, error) {
	var st Status
	if err := c.get(ctx, "/api/status", &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (c *Client) Address(ctx context.Context, addr string) (*AddressView, error) {
	var v AddressView
	if err := c.get(ctx, "/api/address/"+url.PathEscape(addr), &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) DogeWatch(ctx context.Context, addr string) error {
	return c.post(ctx, "/api/doge/watch", map[string]string{"address": addr}, &map[string]any{})
}

func (c *Client) DogeAddress(ctx context.Context, addr string) (*AddressView, error) {
	var v AddressView
	if err := c.get(ctx, "/api/doge/address/"+url.PathEscape(addr), &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) RawTx(ctx context.Context, chain, txid string) (string, error) {
	path := "/api/rawtx/" + url.PathEscape(txid)
	if chain == "dogecoin" {
		path = "/api/doge/rawtx/" + url.PathEscape(txid)
	}
	var out struct {
		Hex string `json:"hex"`
	}
	if err := c.get(ctx, path, &out); err != nil {
		return "", err
	}
	return out.Hex, nil
}

func (c *Client) Broadcast(ctx context.Context, chain, hexTx string) (string, error) {
	path := "/api/tx"
	if chain == "dogecoin" {
		path = "/api/doge/tx"
	}
	var out struct {
		TxID string `json:"txid"`
	}
	if err := c.post(ctx, path, map[string]string{"hex": hexTx}, &out); err != nil {
		return "", err
	}
	return out.TxID, nil
}

func (c *Client) Tx(ctx context.Context, txid string) (*TxView, error) {
	var v TxView
	if err := c.get(ctx, "/api/tx/"+url.PathEscape(txid), &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) DogeTx(ctx context.Context, txid string) (*TxView, error) {
	var v TxView
	if err := c.get(ctx, "/api/doge/tx/"+url.PathEscape(txid), &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) DepositAddress(ctx context.Context, addr string) (*DepositAddressResp, error) {
	var out DepositAddressResp
	if err := c.post(ctx, "/api/deposit-address", map[string]string{"address": addr}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Deposits(ctx context.Context, addr string) ([]Deposit, error) {
	var out []Deposit
	if err := c.get(ctx, "/api/deposits/"+url.PathEscape(addr), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) PegOut(ctx context.Context, vmTxid string) (*PegOut, error) {
	var out PegOut
	if err := c.get(ctx, "/api/pegout/"+url.PathEscape(vmTxid), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Activity(ctx context.Context) ([]ActivityItem, error) {
	var out []ActivityItem
	if err := c.get(ctx, "/api/activity", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) DogeImport(ctx context.Context, addr, txid string) error {
	return c.post(ctx, "/api/doge/import", map[string]string{
		"address": addr,
		"txid":    txid,
	}, &map[string]any{})
}

// OpenEvents returns the SSE body reader for /api/events. Caller must Close.
func (c *Client) OpenEvents(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/events", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "dogevm-connect/1.0")
	// No overall timeout for long-lived streams.
	httpClient := &http.Client{Timeout: 0}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &APIError{Status: resp.StatusCode, Msg: string(body)}
	}
	return resp.Body, nil
}
