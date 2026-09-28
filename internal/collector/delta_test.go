package collector

import "testing"

func TestDeltaWrap(t *testing.T) {
	if got := deltaCounter(^uint64(0)-2, 1); got != 4 {
		t.Fatalf("got %d", got)
	}
}
func TestCPUPercent(t *testing.T) {
	v, ok := cpuPercent(80, 100, 90, 120)
	if !ok || v != 50 {
		t.Fatalf("got %v %v", v, ok)
	}
}
