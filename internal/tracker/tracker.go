// Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
//
// SPDX-License-Identifier: MIT

package tracker

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/DogecoinVM/dogevm-connect/internal/metalapi"
)

// DogecoinReferenceMs is the nominal Dogecoin mainnet block / header interval (~1 min).
// All speed scores are relative to this: score rises when a chain is much faster.
const DogecoinReferenceMs = 60_000.0

// Sample is one DogecoinVM finality reading.
type Sample struct {
	At         time.Time `json:"at"`
	MedianMs   int       `json:"medianMs"`
	P90Ms      int       `json:"p90Ms"`
	LatestMs   int       `json:"latestMs"`
	Payments   int       `json:"payments"`
	VMHeight   int       `json:"vmHeight"`
	DogeHeight int       `json:"dogeHeight"`
}

// Event is a dual-chain tip update from /api/events.
type Event struct {
	Chain  string    `json:"chain"`
	Height int       `json:"height"`
	At     time.Time `json:"at"`
}

// HeaderGap is the observed time between successive tip heights on one chain.
type HeaderGap struct {
	Chain      string    `json:"chain"`
	FromHeight int       `json:"fromHeight"`
	ToHeight   int       `json:"toHeight"`
	DeltaMs    int64     `json:"deltaMs"`
	At         time.Time `json:"at"`
}

// Tracker polls status for VM finality and follows SSE tips to measure
// Dogecoin mainnet header cadence (time between new headers).
type Tracker struct {
	client *metalapi.Client

	mu          sync.RWMutex
	samples     []Sample
	events      []Event
	dogeGaps    []HeaderGap
	vmGaps      []HeaderGap
	lastDogeTip *Event
	lastVMTip   *Event
	last        *metalapi.Status
	running     bool
}

func New(client *metalapi.Client) *Tracker {
	return &Tracker{
		client:   client,
		samples:  make([]Sample, 0, 120),
		events:   make([]Event, 0, 80),
		dogeGaps: make([]HeaderGap, 0, 60),
		vmGaps:   make([]HeaderGap, 0, 60),
	}
}

func (t *Tracker) Start(ctx context.Context) {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	t.running = true
	t.mu.Unlock()

	go t.pollLoop(ctx)
	go t.eventsLoop(ctx)
}

func (t *Tracker) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()
	t.sampleOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.sampleOnce(ctx)
		}
	}
}

func (t *Tracker) sampleOnce(ctx context.Context) {
	st, err := t.client.Status(ctx)
	if err != nil {
		log.Printf("tracker status: %v", err)
		return
	}
	s := Sample{
		At:         time.Now().UTC(),
		MedianMs:   st.Finality.MedianMs,
		P90Ms:      st.Finality.P90Ms,
		LatestMs:   st.Finality.LatestMs,
		Payments:   st.Finality.Payments,
		VMHeight:   st.DogecoinVMHeight,
		DogeHeight: st.DogecoinHeight,
	}
	t.mu.Lock()
	t.last = st
	t.samples = append(t.samples, s)
	if len(t.samples) > 180 {
		t.samples = t.samples[len(t.samples)-180:]
	}
	t.mu.Unlock()
}

func (t *Tracker) eventsLoop(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := t.consumeEvents(ctx)
		if ctx.Err() != nil {
			return
		}
		log.Printf("tracker events reconnect in %s: %v", backoff, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (t *Tracker) consumeEvents(ctx context.Context) error {
	body, err := t.client.OpenEvents(ctx)
	if err != nil {
		return err
	}
	defer body.Close()

	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventName string
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if eventName == "" || eventName == "block" {
				var payload struct {
					Chain  string `json:"chain"`
					Height int    `json:"height"`
				}
				if json.Unmarshal([]byte(data), &payload) == nil && payload.Chain != "" {
					t.recordTip(payload.Chain, payload.Height)
				}
			}
			eventName = ""
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return sc.Err()
}

func (t *Tracker) recordTip(chain string, height int) {
	now := time.Now().UTC()
	ev := Event{Chain: chain, Height: height, At: now}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.events = append(t.events, ev)
	if len(t.events) > 100 {
		t.events = t.events[len(t.events)-100:]
	}

	switch chain {
	case "dogecoin":
		if t.lastDogeTip != nil && height > t.lastDogeTip.Height {
			gap := HeaderGap{
				Chain:      chain,
				FromHeight: t.lastDogeTip.Height,
				ToHeight:   height,
				DeltaMs:    now.Sub(t.lastDogeTip.At).Milliseconds(),
				At:         now,
			}
			if gap.DeltaMs > 0 && gap.DeltaMs < 15*60*1000 {
				t.dogeGaps = append(t.dogeGaps, gap)
				if len(t.dogeGaps) > 60 {
					t.dogeGaps = t.dogeGaps[len(t.dogeGaps)-60:]
				}
			}
		}
		t.lastDogeTip = &ev
	case "dogecoinvm":
		if t.lastVMTip != nil && height > t.lastVMTip.Height {
			gap := HeaderGap{
				Chain:      chain,
				FromHeight: t.lastVMTip.Height,
				ToHeight:   height,
				DeltaMs:    now.Sub(t.lastVMTip.At).Milliseconds(),
				At:         now,
			}
			if gap.DeltaMs > 0 && gap.DeltaMs < 15*60*1000 {
				t.vmGaps = append(t.vmGaps, gap)
				if len(t.vmGaps) > 60 {
					t.vmGaps = t.vmGaps[len(t.vmGaps)-60:]
				}
			}
		}
		t.lastVMTip = &ev
	}
}

func (t *Tracker) Snapshot() (samples []Sample, events []Event, last *metalapi.Status) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	samples = append([]Sample(nil), t.samples...)
	events = append([]Event(nil), t.events...)
	last = t.last
	return
}

func medianInt64(vals []int64) int64 {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]int64(nil), vals...)
	for i := 0; i < len(cp); i++ {
		for j := i + 1; j < len(cp); j++ {
			if cp[j] < cp[i] {
				cp[i], cp[j] = cp[j], cp[i]
			}
		}
	}
	return cp[len(cp)/2]
}

// speedFromDuration maps how long an action takes into a 0-100 gauge on a
// shared scale keyed to Dogecoin's ~60s header interval.
//
//	1× (60s)   → ~12  (mainnet sits low)
//	~3×        → ~25
//	~30×       → ~55
//	~300× (200ms VM) → ~95  (VM sits high)
func speedFromDuration(durationMs float64) (gauge float64, multiplier float64) {
	if durationMs <= 0 {
		return 0, 0
	}
	multiplier = DogecoinReferenceMs / durationMs
	// Cap the log domain around ~400× so ~300× lands near the top.
	maxLog := math.Log10(1 + 400)
	gauge = 100 * math.Log10(1+multiplier) / maxLog
	if gauge > 100 {
		gauge = 100
	}
	if gauge < 0 {
		gauge = 0
	}
	return gauge, multiplier
}

func (t *Tracker) Speed() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var latest Sample
	if len(t.samples) > 0 {
		latest = t.samples[len(t.samples)-1]
	}

	vmFinalityMs := float64(latest.MedianMs)
	if vmFinalityMs <= 0 {
		// Typical DogecoinVM finality until first sample (~200ms ≈ 300×).
		vmFinalityMs = 200
	}
	vmGauge, vmMult := speedFromDuration(vmFinalityMs)

	dogeIntervals := make([]int64, 0, len(t.dogeGaps))
	for _, g := range t.dogeGaps {
		dogeIntervals = append(dogeIntervals, g.DeltaMs)
	}
	dogeMedianGap := medianInt64(dogeIntervals)
	var dogeLatestGap int64
	if len(t.dogeGaps) > 0 {
		dogeLatestGap = t.dogeGaps[len(t.dogeGaps)-1].DeltaMs
	}
	waitingTips := len(dogeIntervals) == 0
	if dogeMedianGap == 0 {
		dogeMedianGap = int64(DogecoinReferenceMs)
	}
	dogeGauge, dogeMult := speedFromDuration(float64(dogeMedianGap))

	return map[string]any{
		"dogecoinvm": map[string]any{
			"label":       "DogecoinVM",
			"gauge":       vmGauge,
			"multiplier":  vmMult,
			"medianMs":    latest.MedianMs,
			"p90Ms":       latest.P90Ms,
			"latestMs":    latest.LatestMs,
			"durationMs":  vmFinalityMs,
			"payments":    latest.Payments,
			"height":      latest.VMHeight,
			"metric":      "payment finality",
			"samples":     append([]Sample(nil), t.samples...),
			"headerGaps":  append([]HeaderGap(nil), t.vmGaps...),
			"referenceMs": DogecoinReferenceMs,
		},
		"dogecoin": map[string]any{
			"label":       "Dogecoin mainnet",
			"gauge":       dogeGauge,
			"multiplier":  dogeMult,
			"medianMs":    dogeMedianGap,
			"latestMs":    dogeLatestGap,
			"durationMs":  dogeMedianGap,
			"height":      latest.DogeHeight,
			"metric":      "header interval",
			"samples":     len(dogeIntervals),
			"headerGaps":  append([]HeaderGap(nil), t.dogeGaps...),
			"nominalMs":   DogecoinReferenceMs,
			"referenceMs": DogecoinReferenceMs,
			"waitingTips": waitingTips,
		},
		"gauge":      vmGauge,
		"medianMs":   latest.MedianMs,
		"p90Ms":      latest.P90Ms,
		"latestMs":   latest.LatestMs,
		"payments":   latest.Payments,
		"vmHeight":   latest.VMHeight,
		"dogeHeight": latest.DogeHeight,
		"samples":    append([]Sample(nil), t.samples...),
		"tipEvents":  append([]Event(nil), t.events...),
		"comparedTo": map[string]any{
			"dogecoinHeaderMs": dogeMedianGap,
			"vmMultiplier":     vmMult,
			"dogeMultiplier":   dogeMult,
			"note":             "Shared scale vs ~60s Dogecoin headers; ~300× VM finality pins near the top",
		},
	}
}
