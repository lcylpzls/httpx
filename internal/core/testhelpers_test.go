package core

import (
	"context"
	"sync"

	"github.com/lcylpzls/logx"
)

// fakeLogger 是 logx.Logger 的内存实现,用于断言日志输出。
type fakeLogger struct {
	mu        sync.Mutex
	debugMsgs []string
	warnMsgs  []string
	errorMsgs []string
}

func (l *fakeLogger) IsDebugEnabled() bool { return true }

func (l *fakeLogger) Debug(msg string, _ logx.FieldGroup) {
	l.mu.Lock()
	l.debugMsgs = append(l.debugMsgs, msg)
	l.mu.Unlock()
}

func (l *fakeLogger) Info(string, logx.FieldGroup) {}

func (l *fakeLogger) Warn(msg string, _ logx.FieldGroup) {
	l.mu.Lock()
	l.warnMsgs = append(l.warnMsgs, msg)
	l.mu.Unlock()
}

func (l *fakeLogger) Error(msg string, _ logx.FieldGroup) {
	l.mu.Lock()
	l.errorMsgs = append(l.errorMsgs, msg)
	l.mu.Unlock()
}

func (l *fakeLogger) Panic(string, logx.FieldGroup) {}
func (l *fakeLogger) Fatal(string, logx.FieldGroup) {}

func (l *fakeLogger) Debugf(string, ...any) {}
func (l *fakeLogger) Infof(string, ...any)  {}
func (l *fakeLogger) Warnf(string, ...any)  {}
func (l *fakeLogger) Errorf(string, ...any) {}
func (l *fakeLogger) Panicf(string, ...any) {}
func (l *fakeLogger) Fatalf(string, ...any) {}

func (l *fakeLogger) WithContext(context.Context) logx.Logger { return l }
func (l *fakeLogger) WithField(string, any) logx.Logger       { return l }
func (l *fakeLogger) Sync() error                             { return nil }
func (l *fakeLogger) Close() error                            { return nil }
func (l *fakeLogger) SafeExit(func())                         {}

func (l *fakeLogger) hasDebug(msg string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, m := range l.debugMsgs {
		if m == msg {
			return true
		}
	}
	return false
}

func (l *fakeLogger) hasWarn(msg string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, m := range l.warnMsgs {
		if m == msg {
			return true
		}
	}
	return false
}

// fakeMetrics 是 Metrics 的内存实现,用于断言指标输出。
type fakeMetrics struct {
	mu        sync.Mutex
	counters  map[string]int
	durations map[string][]float64
}

func newFakeMetrics() *fakeMetrics {
	return &fakeMetrics{
		counters:  make(map[string]int),
		durations: make(map[string][]float64),
	}
}

func (m *fakeMetrics) IncCounter(name string, labels []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := name
	for _, l := range labels {
		key += "|" + l
	}
	m.counters[key]++
}

func (m *fakeMetrics) AddCounter(name string, delta float64, labels []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := name
	for _, l := range labels {
		key += "|" + l
	}
	m.counters[key] += int(delta)
}

func (m *fakeMetrics) ObserveDuration(name string, seconds float64, labels []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := name
	for _, l := range labels {
		key += "|" + l
	}
	m.durations[key] = append(m.durations[key], seconds)
}

func (m *fakeMetrics) AddGauge(name string, delta float64, labels []string) {}

func (m *fakeMetrics) SetGauge(name string, value float64, labels []string) {}

func (m *fakeMetrics) RegisterMetric(name, help string, labelNames []string) error {
	return nil
}

func (m *fakeMetrics) counter(name string, labels ...string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := name
	for _, l := range labels {
		key += "|" + l
	}
	return m.counters[key]
}
