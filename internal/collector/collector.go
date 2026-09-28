package collector

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Darkriqu/Server-Monitor/internal/model"
)

type cpuTicks struct{ idle, total uint64 }
type diskRaw struct{ reads, writes, sectorsRead, sectorsWritten uint64 }
type netRaw struct{ rxBytes, txBytes uint64 }
type procRaw struct{ ticks uint64 }

type Collector struct {
	prevCPU    []cpuTicks
	prevDisk   map[string]diskRaw
	prevNet    map[string]netRaw
	prevProc   map[int]procRaw
	prevAt     time.Time
	clockTicks float64
	pageSize   uint64
}

func New() *Collector {
	return &Collector{prevDisk: map[string]diskRaw{}, prevNet: map[string]netRaw{}, prevProc: map[int]procRaw{}, clockTicks: 100, pageSize: uint64(os.Getpagesize())}
}

func System() (model.SystemInfo, error) {
	host, _ := os.Hostname()
	kernel := readTrim("/proc/sys/kernel/osrelease")
	osName := readOSName()
	modelName, cores := cpuModel()
	up, err := uptime()
	if err != nil {
		return model.SystemInfo{}, err
	}
	return model.SystemInfo{Hostname: host, OS: osName, Kernel: kernel, Arch: runtime.GOARCH, CPUModel: modelName, CPUCores: cores, UptimeSec: uint64(up.Seconds()), BootTime: time.Now().Add(-up).Unix()}, nil
}

func (c *Collector) Collect(now time.Time) (model.Snapshot, error) {
	cpus, err := readCPU()
	if err != nil {
		return model.Snapshot{}, err
	}
	cpu := model.CPU{PerCore: make([]*float64, max(0, len(cpus)-1))}
	if len(c.prevCPU) == len(cpus) {
		if v, ok := cpuPercent(c.prevCPU[0].idle, c.prevCPU[0].total, cpus[0].idle, cpus[0].total); ok {
			cpu.Total = ptr(v)
		}
		for i := 1; i < len(cpus); i++ {
			if v, ok := cpuPercent(c.prevCPU[i].idle, c.prevCPU[i].total, cpus[i].idle, cpus[i].total); ok {
				cpu.PerCore[i-1] = ptr(v)
			}
		}
	}
	c.prevCPU = cpus
	cpu.Load1, cpu.Load5, cpu.Load15 = loadAvg()
	cpu.FreqMHz = frequencies()
	cpu.TempC = temperature()
	mem := memory()
	mounts := mounts()
	elapsed := now.Sub(c.prevAt).Seconds()
	disks, rawDisks := diskIO(c.prevDisk, elapsed)
	nets, rawNets := network(c.prevNet, elapsed)
	c.prevDisk = rawDisks
	c.prevNet = rawNets
	procs, count := c.processes(cpus[0].total)
	established := establishedCount()
	c.prevAt = now
	return model.Snapshot{Timestamp: now.UTC(), CPU: cpu, Memory: mem, Mounts: mounts, DiskIO: disks, Network: nets, Established: established, ProcessCount: count, TopCPU: top(procs, true), TopRAM: top(procs, false)}, nil
}

func readCPU() ([]cpuTicks, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []cpuTicks
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if !strings.HasPrefix(line, "cpu") {
			break
		}
		fs := strings.Fields(line)
		if len(fs) < 5 {
			continue
		}
		var vals []uint64
		for _, x := range fs[1:] {
			v, _ := strconv.ParseUint(x, 10, 64)
			vals = append(vals, v)
		}
		var total uint64
		for _, v := range vals {
			total += v
		}
		idle := vals[3]
		if len(vals) > 4 {
			idle += vals[4]
		}
		out = append(out, cpuTicks{idle, total})
	}
	if len(out) == 0 {
		return nil, errors.New("no cpu data")
	}
	return out, s.Err()
}
func loadAvg() (float64, float64, float64) {
	b, _ := os.ReadFile("/proc/loadavg")
	f := strings.Fields(string(b))
	if len(f) < 3 {
		return 0, 0, 0
	}
	a, _ := strconv.ParseFloat(f[0], 64)
	d, _ := strconv.ParseFloat(f[1], 64)
	e, _ := strconv.ParseFloat(f[2], 64)
	return a, d, e
}
func frequencies() []float64 {
	matches, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq")
	sort.Strings(matches)
	out := make([]float64, 0, len(matches))
	for _, p := range matches {
		v, _ := strconv.ParseFloat(readTrim(p), 64)
		if v > 0 {
			out = append(out, v/1000)
		}
	}
	return out
}
func temperature() *float64 {
	matches, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, p := range matches {
		v, err := strconv.ParseFloat(readTrim(p), 64)
		if err == nil && v > 0 {
			if v > 1000 {
				v /= 1000
			}
			if v > -50 && v < 200 {
				return ptr(v)
			}
		}
	}
	return nil
}
func memory() model.Memory {
	vals := map[string]uint64{}
	f, err := os.Open("/proc/meminfo")
	if err == nil {
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			parts := strings.Fields(s.Text())
			if len(parts) >= 2 {
				v, _ := strconv.ParseUint(parts[1], 10, 64)
				vals[strings.TrimSuffix(parts[0], ":")] = v * 1024
			}
		}
	}
	total := vals["MemTotal"]
	avail := vals["MemAvailable"]
	used := uint64(0)
	if total > avail {
		used = total - avail
	}
	swapUsed := uint64(0)
	if vals["SwapTotal"] > vals["SwapFree"] {
		swapUsed = vals["SwapTotal"] - vals["SwapFree"]
	}
	pct := 0.0
	if total > 0 {
		pct = float64(used) * 100 / float64(total)
	}
	return model.Memory{Total: total, Used: used, Available: avail, Buffers: vals["Buffers"], Cache: vals["Cached"] + vals["SReclaimable"], SwapTotal: vals["SwapTotal"], SwapUsed: swapUsed, UsedPct: pct}
}
func mounts() []model.Mount {
	b, err := os.ReadFile("/proc/self/mounts")
	if err != nil {
		return nil
	}
	var out []model.Mount
	seen := map[string]bool{}
	skip := map[string]bool{"proc": true, "sysfs": true, "tmpfs": true, "devtmpfs": true, "devpts": true, "cgroup2": true, "overlay": false, "squashfs": true, "tracefs": true, "debugfs": true, "securityfs": true, "pstore": true}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || seen[f[1]] || skip[f[2]] {
			continue
		}
		seen[f[1]] = true
		var st syscall.Statfs_t
		if syscall.Statfs(f[1], &st) != nil {
			continue
		}
		total := st.Blocks * uint64(st.Bsize)
		free := st.Bavail * uint64(st.Bsize)
		used := total - free
		pct := 0.0
		if total > 0 {
			pct = float64(used) * 100 / float64(total)
		}
		it := st.Files
		iu := uint64(0)
		if it > st.Ffree {
			iu = it - st.Ffree
		}
		out = append(out, model.Mount{Device: f[0], MountPoint: f[1], FSType: f[2], Total: total, Used: used, Free: free, UsedPct: pct, InodesTotal: it, InodesUsed: iu})
	}
	return out
}
func diskIO(prev map[string]diskRaw, elapsed float64) ([]model.DiskIO, map[string]diskRaw) {
	b, err := os.ReadFile("/proc/diskstats")
	raw := map[string]diskRaw{}
	if err != nil {
		return nil, raw
	}
	var out []model.DiskIO
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 14 {
			continue
		}
		name := f[2]
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		r := diskRaw{reads: u64(f[3]), sectorsRead: u64(f[5]), writes: u64(f[7]), sectorsWritten: u64(f[9])}
		raw[name] = r
		d := model.DiskIO{Device: name}
		if p, ok := prev[name]; ok && elapsed > 0 {
			rb := float64(deltaCounter(p.sectorsRead, r.sectorsRead)*512) / elapsed
			wb := float64(deltaCounter(p.sectorsWritten, r.sectorsWritten)*512) / elapsed
			ri := float64(deltaCounter(p.reads, r.reads)) / elapsed
			wi := float64(deltaCounter(p.writes, r.writes)) / elapsed
			d.ReadBps = &rb
			d.WriteBps = &wb
			d.ReadIOPS = &ri
			d.WriteIOPS = &wi
		}
		out = append(out, d)
	}
	return out, raw
}
func network(prev map[string]netRaw, elapsed float64) ([]model.Network, map[string]netRaw) {
	b, err := os.ReadFile("/proc/net/dev")
	raw := map[string]netRaw{}
	if err != nil {
		return nil, raw
	}
	var out []model.Network
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		name := strings.TrimSpace(parts[0])
		f := strings.Fields(parts[1])
		if len(f) < 16 {
			continue
		}
		r := netRaw{rxBytes: u64(f[0]), txBytes: u64(f[8])}
		raw[name] = r
		n := model.Network{Interface: name, RxPackets: u64(f[1]), RxErrors: u64(f[2]), RxDrops: u64(f[3]), TxPackets: u64(f[9]), TxErrors: u64(f[10]), TxDrops: u64(f[11])}
		if p, ok := prev[name]; ok && elapsed > 0 {
			rx := float64(deltaCounter(p.rxBytes, r.rxBytes)*8) / elapsed
			tx := float64(deltaCounter(p.txBytes, r.txBytes)*8) / elapsed
			n.RxBps = &rx
			n.TxBps = &tx
		}
		out = append(out, n)
	}
	return out, raw
}
func (c *Collector) processes(systemTotal uint64) ([]model.Process, int) {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil, 0
	}
	next := map[int]procRaw{}
	var out []model.Process
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || !e.IsDir() {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		s := string(stat)
		l := strings.Index(s, "(")
		r := strings.LastIndex(s, ")")
		if l < 0 || r < 0 || r+2 >= len(s) {
			continue
		}
		name := s[l+1 : r]
		f := strings.Fields(s[r+2:])
		if len(f) < 18 {
			continue
		}
		ticks := u64(f[11]) + u64(f[12])
		threads, _ := strconv.Atoi(f[17])
		next[pid] = procRaw{ticks: ticks}
		cpu := 0.0
		if p, ok := c.prevProc[pid]; ok && len(c.prevCPU) > 0 {
			pd := deltaCounter(p.ticks, ticks)
			sd := deltaCounter(c.prevCPU[0].total, systemTotal)
			if sd > 0 {
				cpu = float64(pd) * 100 * float64(max(1, runtime.NumCPU())) / float64(sd)
			}
		}
		rss := readRSS(pid, c.pageSize)
		out = append(out, model.Process{PID: pid, Name: name, CPUPercent: cpu, RSS: rss, Threads: threads})
	}
	c.prevProc = next
	return out, len(out)
}
func readRSS(pid int, page uint64) uint64 {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) < 2 {
		return 0
	}
	return u64(f[1]) * page
}
func top(in []model.Process, byCPU bool) []model.Process {
	cp := append([]model.Process(nil), in...)
	sort.Slice(cp, func(i, j int) bool {
		if byCPU {
			return cp[i].CPUPercent > cp[j].CPUPercent
		}
		return cp[i].RSS > cp[j].RSS
	})
	if len(cp) > 10 {
		cp = cp[:10]
	}
	return cp
}
func establishedCount() int { return countTCP("/proc/net/tcp") + countTCP("/proc/net/tcp6") }
func countTCP(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for i, line := range strings.Split(string(b), "\n") {
		if i == 0 {
			continue
		}
		f := strings.Fields(line)
		if len(f) > 3 && f[3] == "01" {
			n++
		}
	}
	return n
}
func uptime() (time.Duration, error) {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0, errors.New("invalid /proc/uptime")
	}
	v, err := strconv.ParseFloat(f[0], 64)
	return time.Duration(v * float64(time.Second)), err
}
func cpuModel() (string, int) {
	b, _ := os.ReadFile("/proc/cpuinfo")
	modelName := "unknown"
	cores := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "processor") {
			cores++
		}
		if modelName == "unknown" && strings.HasPrefix(line, "model name") {
			if p := strings.SplitN(line, ":", 2); len(p) == 2 {
				modelName = strings.TrimSpace(p[1])
			}
		}
	}
	if cores == 0 {
		cores = runtime.NumCPU()
	}
	return modelName, cores
}
func readOSName() string {
	b, _ := os.ReadFile("/etc/os-release")
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
	}
	return runtime.GOOS
}
func readTrim(p string) string { b, _ := os.ReadFile(p); return strings.TrimSpace(string(b)) }
func u64(s string) uint64      { v, _ := strconv.ParseUint(s, 10, 64); return v }
func ptr(v float64) *float64   { return &v }
