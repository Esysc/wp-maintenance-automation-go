package metrics

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

// Host describes resource usage and identification of the Docker host machine.
type Host struct {
	Hostname      string      `json:"hostname"`
	Platform      string      `json:"platform"`
	OS            string      `json:"os"`
	Arch          string      `json:"arch"`
	Kernel        string      `json:"kernel"`
	UptimeSeconds int64       `json:"uptime_seconds"`
	CPUs          int         `json:"cpus"`
	LoadAvg       []float64   `json:"load_avg"`
	CPUPercent    float64     `json:"cpu_percent"`
	MemoryTotal   uint64      `json:"memory_total"`
	MemoryUsed    uint64      `json:"memory_used"`
	MemoryPercent float64     `json:"memory_percent"`
	DiskTotal     uint64      `json:"disk_total"`
	DiskUsed      uint64      `json:"disk_used"`
	DiskPercent   float64     `json:"disk_percent"`
	Available     bool        `json:"available"`
	Docker        *DockerHost `json:"docker,omitempty"`
}

// DockerHost describes the Docker daemon/host as reported by `docker info`.
type DockerHost struct {
	Name            string `json:"name"`
	OperatingSystem string `json:"operating_system"`
	OSType          string `json:"os_type"`
	Architecture    string `json:"architecture"`
	KernelVersion   string `json:"kernel_version"`
	ServerVersion   string `json:"server_version"`
	DockerRootDir   string `json:"docker_root_dir"`
	NCPU            int    `json:"ncpu"`
	MemTotal        uint64 `json:"mem_total"`
}

// Container describes a container of the application's docker-compose project.
type Container struct {
	Name          string  `json:"name"`
	Image         string  `json:"image"`
	State         string  `json:"state"`
	Status        string  `json:"status"`
	RunningFor    string  `json:"running_for"`
	Ports         string  `json:"ports"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsed    uint64  `json:"memory_used,omitempty"`
	MemoryLimit   uint64  `json:"memory_limit,omitempty"`
	MemoryPercent float64 `json:"memory_percent,omitempty"`
}

const dockerTimeout = 10 * time.Second

// CollectHost collects resource usage and identification of the machine the
// current process runs on, using best-effort reads: fields that cannot be read
// are left at their zero value and Available is set to false. Unlike
// HostMetrics it does not query the Docker daemon.
func CollectHost() Host {
	h := Host{
		CPUs: runtime.NumCPU(),
	}

	if info, err := host.Info(); err == nil {
		h.Hostname = info.Hostname
		h.Platform = strings.TrimSpace(info.Platform + " " + info.PlatformVersion)
		h.OS = info.OS
		h.Arch = info.KernelArch
		h.Kernel = info.KernelVersion
		h.UptimeSeconds = int64(info.Uptime)
		h.Available = true
	}

	if avg, err := load.Avg(); err == nil {
		h.LoadAvg = []float64{avg.Load1, avg.Load5, avg.Load15}
	}

	if pct, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(pct) > 0 {
		h.CPUPercent = round1(pct[0])
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		h.MemoryTotal = vm.Total
		h.MemoryUsed = vm.Used
		h.MemoryPercent = round1(vm.UsedPercent)
	}

	if du, err := disk.Usage("/"); err == nil {
		h.DiskTotal = du.Total
		h.DiskUsed = du.Used
		h.DiskPercent = round1(du.UsedPercent)
	}

	return h
}

// HostMetrics collects resource usage of the machine backing the app. It
// prefers a host agent running on the physical host when HOST_AGENT_URL is
// configured; otherwise it reports the Docker host as seen from this
// container. The Docker daemon remains the authoritative source for the
// Docker host's identity and capacity.
func HostMetrics() Host {
	h := CollectHost()

	if d, err := dockerHostInfo(); err == nil {
		h.Docker = d
		h.Hostname = d.Name
		h.Platform = d.OperatingSystem
		h.OS = d.OSType
		h.Arch = d.Architecture
		h.Kernel = d.KernelVersion
		if d.NCPU > 0 {
			h.CPUs = d.NCPU
		}
		if d.MemTotal > 0 {
			h.MemoryTotal = d.MemTotal
			if h.MemoryUsed <= h.MemoryTotal {
				h.MemoryPercent = round1(float64(h.MemoryUsed) / float64(h.MemoryTotal) * 100)
			}
		}
		h.Available = true
	}

	// Under VM-based Docker hosts (e.g. Colima, Docker Desktop) the Docker
	// host is a VM, not the physical machine. A host agent running on the
	// physical host provides its real metrics when configured.
	if url := os.Getenv("HOST_AGENT_URL"); url != "" {
		if a, err := fetchHostAgent(url); err == nil {
			a.Docker = h.Docker
			return *a
		}
	}

	return h
}

// fetchHostAgent retrieves host metrics from a host agent exposing the Host
// JSON shape at <url>/metrics.
func fetchHostAgent(url string) (*Host, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(url, "/")+"/metrics", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("host agent: unexpected status %d", resp.StatusCode)
	}

	var h Host
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, fmt.Errorf("host agent: decode: %w", err)
	}
	return &h, nil
}

// Containers lists the containers of the docker-compose project this app runs
// under, together with live CPU/memory usage where available. The boolean
// reports whether the app is running under docker-compose at all.
func Containers() ([]Container, bool, error) {
	project := composeProject()
	if project == "" {
		return []Container{}, false, nil
	}

	containers, err := listContainers("label=com.docker.compose.project=" + project)
	return containers, true, err
}

func listContainers(filter string) ([]Container, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "ps", "-a", "--no-trunc",
		"--filter", filter,
		"--format", "{{.Names}}\t{{.Image}}\t{{.State}}\t{{.Status}}\t{{.RunningFor}}\t{{.Ports}}").Output()
	if err != nil {
		return nil, err
	}

	var containers []Container
	var running []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < 6 {
			continue
		}
		c := Container{
			Name:       fields[0],
			Image:      fields[1],
			State:      fields[2],
			Status:     fields[3],
			RunningFor: fields[4],
			Ports:      fields[5],
		}
		containers = append(containers, c)
		if c.State == "running" {
			running = append(running, c.Name)
		}
	}

	stats := dockerStats(running)
	for i := range containers {
		if st, ok := stats[containers[i].Name]; ok {
			containers[i].CPUPercent = st.CPUPercent
			containers[i].MemoryUsed = st.MemoryUsed
			containers[i].MemoryLimit = st.MemoryLimit
			if st.MemoryLimit > 0 {
				containers[i].MemoryPercent = round1(float64(st.MemoryUsed) / float64(st.MemoryLimit) * 100)
			}
		}
	}
	return containers, nil
}

type containerStat struct {
	CPUPercent  float64
	MemoryUsed  uint64
	MemoryLimit uint64
}

func dockerStats(names []string) map[string]containerStat {
	if len(names) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()

	args := append([]string{"stats", "--no-stream", "--no-trunc",
		"--format", "{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}"}, names...)
	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return nil
	}

	stats := make(map[string]containerStat)
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < 3 {
			continue
		}
		var st containerStat
		if v, err := strconv.ParseFloat(strings.TrimSuffix(fields[1], "%"), 64); err == nil {
			st.CPUPercent = v
		}
		st.MemoryUsed, st.MemoryLimit = parseMemUsage(fields[2])
		stats[fields[0]] = st
	}
	return stats
}

func dockerHostInfo() (*DockerHost, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "info",
		"--format", "{{.Name}}\t{{.OperatingSystem}}\t{{.OSType}}\t{{.Architecture}}\t{{.KernelVersion}}\t{{.ServerVersion}}\t{{.DockerRootDir}}\t{{.NCPU}}\t{{.MemTotal}}").Output()
	if err != nil {
		return nil, err
	}

	fields := strings.Split(strings.TrimSpace(string(out)), "\t")
	if len(fields) < 9 {
		return nil, fmt.Errorf("unexpected docker info output")
	}

	d := &DockerHost{
		Name:            fields[0],
		OperatingSystem: fields[1],
		OSType:          fields[2],
		Architecture:    fields[3],
		KernelVersion:   fields[4],
		ServerVersion:   fields[5],
		DockerRootDir:   fields[6],
	}
	d.NCPU, _ = strconv.Atoi(strings.TrimSpace(fields[7]))
	d.MemTotal, _ = strconv.ParseUint(strings.TrimSpace(fields[8]), 10, 64)
	return d, nil
}

// composeProject returns the docker-compose project name the current process
// runs under, or an empty string when not part of a compose stack.
func composeProject() string {
	if env := os.Getenv("COMPOSE_PROJECT_NAME"); env != "" {
		return env
	}

	id := selfContainerID()
	if id == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "docker", "inspect", id,
		"--format", "{{index .Config.Labels \"com.docker.compose.project\"}}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// selfContainerID attempts to derive the current container's ID from cgroup
// and mountinfo files. It returns an empty string when not running in a
// container.
func selfContainerID() string {
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if i := strings.Index(line, "/docker/"); i >= 0 {
				if id, ok := trimContainerID(line[i+len("/docker/"):]); ok {
					return id
				}
			}
			if i := strings.Index(line, "docker-"); i >= 0 {
				if id, ok := trimContainerID(line[i+len("docker-"):]); ok {
					return id
				}
			}
		}
	}

	if data, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if i := strings.Index(line, "/containers/"); i >= 0 {
				if id, ok := trimContainerID(line[i+len("/containers/"):]); ok {
					return id
				}
			}
		}
	}
	return ""
}

func trimContainerID(s string) (string, bool) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			i++
			continue
		}
		break
	}
	return s[:i], i >= 12
}

func parseMemUsage(s string) (used, limit uint64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	return parseBytes(strings.TrimSpace(parts[0])), parseBytes(strings.TrimSpace(parts[1]))
}

func parseBytes(s string) uint64 {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
		i++
	}
	if i == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	mult := uint64(1)
	switch strings.ToUpper(strings.TrimSpace(s[i:])) {
	case "B", "":
		mult = 1
	case "KIB", "KB":
		mult = 1 << 10
	case "MIB", "MB":
		mult = 1 << 20
	case "GIB", "GB":
		mult = 1 << 30
	case "TIB", "TB":
		mult = 1 << 40
	}
	return uint64(v * float64(mult))
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
