package model

import "time"

type SystemInfo struct {
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Kernel    string `json:"kernel"`
	Arch      string `json:"arch"`
	CPUModel  string `json:"cpu_model"`
	CPUCores  int    `json:"cpu_cores"`
	UptimeSec uint64 `json:"uptime_sec"`
	BootTime  int64  `json:"boot_time"`
}

type CPU struct {
	Total   *float64   `json:"total_pct"`
	PerCore []*float64 `json:"per_core_pct"`
	Load1   float64    `json:"load_1"`
	Load5   float64    `json:"load_5"`
	Load15  float64    `json:"load_15"`
	FreqMHz []float64  `json:"freq_mhz"`
	TempC   *float64   `json:"temp_c,omitempty"`
}

type Memory struct {
	Total     uint64  `json:"total_bytes"`
	Used      uint64  `json:"used_bytes"`
	Available uint64  `json:"available_bytes"`
	Buffers   uint64  `json:"buffers_bytes"`
	Cache     uint64  `json:"cache_bytes"`
	SwapTotal uint64  `json:"swap_total_bytes"`
	SwapUsed  uint64  `json:"swap_used_bytes"`
	UsedPct   float64 `json:"used_pct"`
}

type Mount struct {
	Device      string  `json:"device"`
	MountPoint  string  `json:"mount_point"`
	FSType      string  `json:"fs_type"`
	Total       uint64  `json:"total_bytes"`
	Used        uint64  `json:"used_bytes"`
	Free        uint64  `json:"free_bytes"`
	UsedPct     float64 `json:"used_pct"`
	InodesTotal uint64  `json:"inodes_total"`
	InodesUsed  uint64  `json:"inodes_used"`
}

type DiskIO struct {
	Device    string   `json:"device"`
	ReadBps   *float64 `json:"read_bps"`
	WriteBps  *float64 `json:"write_bps"`
	ReadIOPS  *float64 `json:"read_iops"`
	WriteIOPS *float64 `json:"write_iops"`
}

type Network struct {
	Interface string   `json:"interface"`
	RxBps     *float64 `json:"rx_bps"`
	TxBps     *float64 `json:"tx_bps"`
	RxPackets uint64   `json:"rx_packets"`
	TxPackets uint64   `json:"tx_packets"`
	RxErrors  uint64   `json:"rx_errors"`
	TxErrors  uint64   `json:"tx_errors"`
	RxDrops   uint64   `json:"rx_drops"`
	TxDrops   uint64   `json:"tx_drops"`
}

type Process struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu_pct"`
	RSS        uint64  `json:"rss_bytes"`
	Threads    int     `json:"threads"`
}

type Snapshot struct {
	Timestamp    time.Time `json:"timestamp"`
	CPU          CPU       `json:"cpu"`
	Memory       Memory    `json:"memory"`
	Mounts       []Mount   `json:"mounts"`
	DiskIO       []DiskIO  `json:"disk_io"`
	Network      []Network `json:"network"`
	Established  int       `json:"established_connections"`
	ProcessCount int       `json:"process_count"`
	TopCPU       []Process `json:"top_cpu"`
	TopRAM       []Process `json:"top_ram"`
}

type Alert struct {
	ID        string    `json:"id"`
	Rule      string    `json:"rule"`
	State     string    `json:"state"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Message   string    `json:"message"`
}
