package heed

import (
	"fmt"
	"time"
)

type TrendBuffer struct {
	values []float64
	maxLen int
}

func NewTrendBuffer(maxLen int) *TrendBuffer {
	return &TrendBuffer{maxLen: maxLen}
}

func (t *TrendBuffer) Add(value float64) {
	t.values = append(t.values, value)
	if len(t.values) > t.maxLen {
		t.values = t.values[len(t.values)-t.maxLen:]
	}
}

func (t *TrendBuffer) Len() int {
	return len(t.values)
}

func (t *TrendBuffer) IsIncreasing() bool {
	if len(t.values) < 5 {
		return false
	}
	last5 := t.values[len(t.values)-5:]
	for i := 1; i < len(last5); i++ {
		if last5[i] <= last5[i-1] {
			return false
		}
	}
	return true
}

func (t *TrendBuffer) EstimateTimeToThreshold(threshold float64, interval time.Duration) time.Duration {
	if len(t.values) < 2 {
		return 0
	}

	last := t.values[len(t.values)-1]
	if last >= threshold {
		return 0
	}

	first := t.values[0]
	if last <= first {
		return 0
	}

	rate := (last - first) / float64(len(t.values)-1)
	if rate <= 0 {
		return 0
	}

	remaining := threshold - last
	intervals := remaining / rate
	return time.Duration(intervals) * interval
}

func FormatTrendAlert(name string, threshold float64, eta time.Duration) string {
	return fmt.Sprintf("%s increasing continuously, estimated %s to %d%%", name, formatAge(eta), int(threshold))
}
