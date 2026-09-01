package main

// procstat.go — samples resident memory and CPU time for a server process and
// all of its children. One `ps` call per sample keeps the observer cheap enough
// not to distort what it is observing.

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type procSample struct {
	rssKB   int
	cpuSecs float64
}

type memSampler struct {
	pid      int
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}

	mu      sync.Mutex
	peakKB  int
	lastCPU float64
	samples int
}

func startMemSampler(pid int, interval time.Duration) *memSampler {
	m := &memSampler{
		pid:      pid,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	m.sample() // one synchronous sample so Peak() is never empty
	go m.loop()
	return m
}

func (m *memSampler) loop() {
	defer close(m.done)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	m.sample()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.sample()
		}
	}
}

func (m *memSampler) sample() {
	s, ok := treeStats(m.pid)
	if !ok {
		return
	}
	m.mu.Lock()
	if s.rssKB > m.peakKB {
		m.peakKB = s.rssKB
	}
	if s.cpuSecs > m.lastCPU {
		m.lastCPU = s.cpuSecs
	}
	m.samples++
	m.mu.Unlock()
}

// Stop halts sampling and returns peak RSS in MB and total CPU seconds.
func (m *memSampler) Stop() (peakMB float64, cpuSecs float64) {
	m.sample()
	close(m.stop)
	<-m.done
	m.mu.Lock()
	defer m.mu.Unlock()
	return float64(m.peakKB) / 1024, m.lastCPU
}

// Peak returns the peak RSS in MB observed so far without stopping.
func (m *memSampler) Peak() float64 {
	m.sample()
	m.mu.Lock()
	defer m.mu.Unlock()
	return float64(m.peakKB) / 1024
}

// treeStats sums RSS and CPU time over pid and its descendants.
func treeStats(root int) (procSample, bool) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,rss=,time=").Output()
	if err != nil {
		return procSample{}, false
	}
	type row struct {
		ppid int
		rss  int
		cpu  float64
	}
	rows := make(map[int]row)
	children := make(map[int][]int)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		rss, err3 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		rows[pid] = row{ppid: ppid, rss: rss, cpu: parseCPUTime(fields[3])}
		children[ppid] = append(children[ppid], pid)
	}
	if _, ok := rows[root]; !ok {
		return procSample{}, false
	}
	var total procSample
	stack := []int{root}
	seen := map[int]bool{}
	for len(stack) > 0 {
		pid := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		r, ok := rows[pid]
		if !ok {
			continue
		}
		total.rssKB += r.rss
		total.cpuSecs += r.cpu
		stack = append(stack, children[pid]...)
	}
	return total, true
}

// parseCPUTime handles the ps TIME column: "MM:SS.ss", "HH:MM:SS" or "D-HH:MM:SS".
func parseCPUTime(s string) float64 {
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		days, _ := strconv.ParseFloat(s[:idx], 64)
		return days*86400 + parseCPUTime(s[idx+1:])
	}
	parts := strings.Split(s, ":")
	var total float64
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0
		}
		total = total*60 + v
	}
	return total
}
