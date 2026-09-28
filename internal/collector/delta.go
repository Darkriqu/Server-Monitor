package collector

func deltaCounter(prev, curr uint64) uint64 {
	if curr >= prev {
		return curr - prev
	}
	return (^uint64(0) - prev) + curr + 1
}
func cpuPercent(prevIdle, prevTotal, currIdle, currTotal uint64) (float64, bool) {
	dt := deltaCounter(prevTotal, currTotal)
	if dt == 0 {
		return 0, false
	}
	di := deltaCounter(prevIdle, currIdle)
	if di > dt {
		di = dt
	}
	return float64(dt-di) * 100 / float64(dt), true
}
