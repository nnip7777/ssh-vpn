package gui

import (
	"fmt"
	"sync"
	"time"
)

type ThrottleLevel int

const (
	ThrottleNone ThrottleLevel = iota
	ThrottleMild
	ThrottleModerate
	ThrottleSevere
)

func (t ThrottleLevel) String() string {
	switch t {
	case ThrottleMild:
		return "MILD"
	case ThrottleModerate:
		return "MODERATE"
	case ThrottleSevere:
		return "SEVERE"
	default:
		return "NONE"
	}
}

type ThrottleEvent struct {
	Time      time.Time
	ChannelID uint16
	Level     ThrottleLevel
	Reason    string
	Before    float64
	After     float64
}

type channelSample struct {
	bytesSent uint64
	bytesRecv uint64
	time      time.Time
}

type ThrottleDetector struct {
	mu          sync.Mutex
	samples     map[uint16][]channelSample
	prevErrors  map[string]uint64
	cooldowns   map[uint16]time.Time
	maxSamples  int
	cooldown    time.Duration
	warmup      time.Duration
	startedAt   time.Time
	events      []ThrottleEvent
	maxEvents   int
	dropThresh  float64
	errorThresh float64
	latThresh   time.Duration
	listeners   []func(ThrottleEvent)
}

func NewThrottleDetector() *ThrottleDetector {
	return &ThrottleDetector{
		samples:     make(map[uint16][]channelSample),
		prevErrors:  make(map[string]uint64),
		cooldowns:   make(map[uint16]time.Time),
		maxSamples:  120,
		cooldown:    60 * time.Second,
		warmup:      60 * time.Second,
		startedAt:   time.Now(),
		maxEvents:   200,
		dropThresh:  0.4,
		errorThresh: 5.0,
		latThresh:   500 * time.Millisecond,
	}
}

func (td *ThrottleDetector) OnEvent(fn func(ThrottleEvent)) {
	td.mu.Lock()
	defer td.mu.Unlock()
	td.listeners = append(td.listeners, fn)
}

func (td *ThrottleDetector) Analyze(chID uint16, bytesSent, bytesRecv uint64, totalErrors uint64, latency time.Duration) []ThrottleEvent {
	td.mu.Lock()
	defer td.mu.Unlock()

	now := time.Now()

	if now.Sub(td.startedAt) < td.warmup {
		return nil
	}
	sample := channelSample{bytesSent: bytesSent, bytesRecv: bytesRecv, time: now}

	history := td.samples[chID]
	history = append(history, sample)
	if len(history) > td.maxSamples {
		history = history[len(history)-td.maxSamples:]
	}
	td.samples[chID] = history

	var events []ThrottleEvent

	events = append(events, td.checkThroughputDrop(chID, history, now)...)
	events = append(events, td.checkErrorSpike(chID, totalErrors, now)...)
	events = append(events, td.checkLatencySpike(chID, latency, now)...)

	for _, e := range events {
		td.events = append(td.events, e)
		if len(td.events) > td.maxEvents {
			td.events = td.events[len(td.events)-td.maxEvents:]
		}
		for _, fn := range td.listeners {
			fn(e)
		}
	}

	return events
}

func (td *ThrottleDetector) onCooldown(chID uint16, now time.Time) bool {
	if last, ok := td.cooldowns[chID]; ok && now.Sub(last) < td.cooldown {
		return true
	}
	return false
}

func (td *ThrottleDetector) setCooldown(chID uint16, now time.Time) {
	td.cooldowns[chID] = now
}

func (td *ThrottleDetector) checkThroughputDrop(chID uint16, history []channelSample, now time.Time) []ThrottleEvent {
	if td.onCooldown(chID, now) {
		return nil
	}

	if len(history) < 20 {
		return nil
	}

	half := len(history) / 2
	oldSamples := history[:half]
	recentSamples := history[half:]

	if len(recentSamples) < 5 || len(oldSamples) < 5 {
		return nil
	}

	oldThroughput := calcThroughput(oldSamples)
	recentThroughput := calcThroughput(recentSamples)

	if oldThroughput < 1024 {
		return nil
	}

	drop := 1.0 - (recentThroughput / oldThroughput)
	if drop < td.dropThresh {
		return nil
	}

	var level ThrottleLevel
	switch {
	case drop >= 0.8:
		level = ThrottleSevere
	case drop >= 0.6:
		level = ThrottleModerate
	default:
		level = ThrottleMild
	}

	td.setCooldown(chID, now)

	return []ThrottleEvent{{
		Time:      now,
		ChannelID: chID,
		Level:     level,
		Reason:    fmt.Sprintf("throughput drop %.0f%%", drop*100),
		Before:    oldThroughput,
		After:     recentThroughput,
	}}
}

func (td *ThrottleDetector) checkErrorSpike(chID uint16, totalErrors uint64, now time.Time) []ThrottleEvent {
	key := fmt.Sprintf("err_%d", chID)
	if td.prevErrors == nil {
		td.prevErrors = make(map[string]uint64)
	}
	prevErrors, _ := td.prevErrors[key]
	td.prevErrors[key] = totalErrors

	errDelta := totalErrors - prevErrors
	if errDelta == 0 {
		return nil
	}

	history := td.samples[chID]
	if len(history) < 2 {
		return nil
	}

	samplePrev := history[len(history)-2]
	sampleCurr := history[len(history)-1]
	sentDelta := sampleCurr.bytesSent - samplePrev.bytesSent
	if sentDelta == 0 {
		return nil
	}

	errorRate := float64(errDelta) / float64(sentDelta) * 1000
	if errorRate < td.errorThresh {
		return nil
	}

	if td.onCooldown(chID, now) {
		return nil
	}
	td.setCooldown(chID, now)

	return []ThrottleEvent{{
		Time:      now,
		ChannelID: chID,
		Level:     ThrottleModerate,
		Reason:    fmt.Sprintf("error rate spike %.1f/1K pkts", errorRate),
		Before:    0,
		After:     errorRate,
	}}
}

func (td *ThrottleDetector) checkLatencySpike(chID uint16, latency time.Duration, now time.Time) []ThrottleEvent {
	if latency <= td.latThresh {
		return nil
	}

	if td.onCooldown(chID, now) {
		return nil
	}
	td.setCooldown(chID, now)

	var level ThrottleLevel
	switch {
	case latency > 2*time.Second:
		level = ThrottleSevere
	case latency > 1*time.Second:
		level = ThrottleModerate
	default:
		level = ThrottleMild
	}

	return []ThrottleEvent{{
		Time:      now,
		ChannelID: chID,
		Level:     level,
		Reason:    fmt.Sprintf("latency %.0fms", float64(latency.Milliseconds())),
		Before:    0,
		After:     float64(latency.Milliseconds()),
	}}
}

func (td *ThrottleDetector) GetEvents() []ThrottleEvent {
	td.mu.Lock()
	defer td.mu.Unlock()
	out := make([]ThrottleEvent, len(td.events))
	copy(out, td.events)
	return out
}

func (td *ThrottleDetector) GetOverallLevel() ThrottleLevel {
	td.mu.Lock()
	defer td.mu.Unlock()

	maxLevel := ThrottleNone
	cutoff := time.Now().Add(-2 * time.Minute)
	for i := len(td.events) - 1; i >= 0; i-- {
		if td.events[i].Time.Before(cutoff) {
			break
		}
		if td.events[i].Level > maxLevel {
			maxLevel = td.events[i].Level
		}
	}
	return maxLevel
}

func calcThroughput(samples []channelSample) float64 {
	if len(samples) < 2 {
		return 0
	}
	first := samples[0]
	last := samples[len(samples)-1]
	elapsed := last.time.Sub(first.time).Seconds()
	if elapsed < 0.5 {
		return 0
	}
	deltaBytes := float64(last.bytesSent - first.bytesSent)
	if last.bytesRecv > first.bytesRecv {
		d := float64(last.bytesRecv - first.bytesRecv)
		if d > deltaBytes {
			deltaBytes = d
		}
	}
	return deltaBytes / elapsed
}
