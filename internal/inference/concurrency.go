package inference

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// maxIntraOpThreads caps per-chunk ORT threads: sweeps put the single-chunk speedup peak near 5-8 threads, and 4 already reaches ~90% of it.
const maxIntraOpThreads = 4

// intraOpThreads is the ORT per-op thread count set by SetIntraOpThreads; 0 picks Concurrency's default.
var intraOpThreads int

// Concurrency returns ORT intra-op threads per chunk and parallel chunk workers, sized so their product stays near the physical core count.
func Concurrency() (threads, workers int) {
	phys := physicalCores()
	threads = intraOpThreads
	if threads <= 0 {
		threads = min(phys, maxIntraOpThreads)
	}
	return threads, max(1, phys/threads)
}

// SetIntraOpThreads overrides the per-chunk ORT thread count (0 = auto); must be called before any model is loaded.
func SetIntraOpThreads(n int) {
	intraOpThreads = n
}

// physicalCores counts the physical cores this process may run on; SMT siblings add little speed at high CPU cost, so they don't count.
func physicalCores() int {
	if n := linuxPhysicalCores(); n > 0 {
		return min(n, runtime.NumCPU())
	}
	n := runtime.NumCPU()
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "386" {
		n /= 2 // most x86 parts, Intel Macs included, run 2-way SMT; ARM (Apple Silicon) has none
	}
	return max(1, n)
}

// linuxPhysicalCores counts distinct cores among the CPUs in this process's affinity mask, so taskset and cgroup cpusets are honoured; 0 means unknown.
func linuxPhysicalCores() int {
	if runtime.GOOS != "linux" {
		return 0
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	var allowed []int
	for line := range strings.SplitSeq(string(status), "\n") {
		if list, ok := strings.CutPrefix(line, "Cpus_allowed_list:"); ok {
			allowed = parseCPUList(strings.TrimSpace(list))
		}
	}
	cores := make(map[string]bool)
	for _, cpu := range allowed {
		dir := "/sys/devices/system/cpu/cpu" + strconv.Itoa(cpu) + "/topology/"
		siblings, err := os.ReadFile(dir + "core_cpus_list")
		if err != nil {
			if siblings, err = os.ReadFile(dir + "thread_siblings_list"); err != nil {
				return 0
			}
		}
		cores[strings.TrimSpace(string(siblings))] = true
	}
	return len(cores)
}

// parseCPUList parses a kernel CPU list such as "0-3,6,8-9"; it returns nil on malformed input.
func parseCPUList(s string) []int {
	var cpus []int
	for part := range strings.SplitSeq(s, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			return nil
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil {
				return nil
			}
		}
		for c := a; c <= b; c++ {
			cpus = append(cpus, c)
		}
	}
	return cpus
}
