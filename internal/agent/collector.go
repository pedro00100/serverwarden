// Package agent gathers the state of the local server. It only reads: /proc,
// /sys, the Docker socket, smartctl and systemctl. It opens no ports.
package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pedro00100/serverwarden/internal/domain"
)

func Collect(ctx context.Context, cfg *Config) domain.Snapshot {
	s := domain.Snapshot{TakenAt: time.Now().UTC(), Cores: runtime.NumCPU()}
	s.Hostname, _ = os.Hostname()
	s.OS = osName()
	var u syscall.Utsname
	if syscall.Uname(&u) == nil {
		s.Kernel = cString(u.Release[:])
	}
	s.UptimeSec = uptime()
	s.Load = loadAvg()
	s.CPU, s.CPUSteal = cpu(500 * time.Millisecond)
	s.Memory, s.Swap = memory()
	s.Disk = diskUsage("/")
	s.Temperatures = temperatures()
	s.Disks = disks(ctx)
	s.Battery = battery()
	s.Containers = containers(ctx)
	s.FailedUnits = failedUnits(ctx)
	_, err := os.Stat("/run/reboot-required")
	s.RebootRequired = err == nil
	s.Tools = detectTools()
	s.Actions = cfg.actionList()
	s.PanelCommands = cfg.PanelCommands
	s.ImageExport = cfg.ImageExport
	return s
}

func (c *Config) actionList() []domain.Action {
	out := []domain.Action{}
	for id, a := range c.Actions {
		out = append(out, domain.Action{ID: id, Name: a.Name, Description: a.Description})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// cString reads a NUL-terminated utsname field: []int8 on most CPUs, []uint8 on 32-bit ARM.
func cString[T int8 | uint8](b []T) string {
	s := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		s = append(s, byte(c))
	}
	return string(s)
}

func osName() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

func uptime() int64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	f, _ := strconv.ParseFloat(strings.Fields(string(b))[0], 64)
	return int64(f)
}

func loadAvg() [3]float64 {
	var l [3]float64
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return l
	}
	fields := strings.Fields(string(b))
	for k := 0; k < 3 && k < len(fields); k++ {
		l[k], _ = strconv.ParseFloat(fields[k], 64)
	}
	return l
}

// cpu measures usage and steal time (VMs) by sampling /proc/stat twice.
func cpu(interval time.Duration) (usage, steal float64) {
	read := func() []uint64 {
		b, err := os.ReadFile("/proc/stat")
		if err != nil {
			return nil
		}
		line := strings.SplitN(string(b), "\n", 2)[0]
		var v []uint64
		for _, c := range strings.Fields(line)[1:] {
			n, _ := strconv.ParseUint(c, 10, 64)
			v = append(v, n)
		}
		return v
	}
	a := read()
	time.Sleep(interval)
	b := read()
	if len(a) < 8 || len(b) < 8 {
		return 0, 0
	}
	var total uint64
	for k := 0; k < 8; k++ { // user nice system idle iowait irq softirq steal
		total += b[k] - a[k]
	}
	if total == 0 {
		return 0, 0
	}
	idle := (b[3] - a[3]) + (b[4] - a[4])
	usage = float64(total-idle) * 100 / float64(total)
	steal = float64(b[7]-a[7]) * 100 / float64(total)
	return usage, steal
}

func memory() (mem, swap domain.Usage) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()
	v := map[string]uint64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		c := strings.Fields(sc.Text())
		if len(c) >= 2 {
			n, _ := strconv.ParseUint(c[1], 10, 64)
			v[strings.TrimSuffix(c[0], ":")] = n * 1024
		}
	}
	mem = domain.Usage{Total: v["MemTotal"], Used: v["MemTotal"] - v["MemAvailable"]}
	swap = domain.Usage{Total: v["SwapTotal"], Used: v["SwapTotal"] - v["SwapFree"]}
	return
}

func diskUsage(path string) domain.Usage {
	var s syscall.Statfs_t
	if syscall.Statfs(path, &s) != nil {
		return domain.Usage{}
	}
	bs := uint64(s.Bsize)
	return domain.Usage{Total: s.Blocks * bs, Used: (s.Blocks - s.Bfree) * bs}
}

func temperatures() []domain.Temperature {
	out := []domain.Temperature{}
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, d := range dirs {
		sensor := readText(filepath.Join(d, "name"))
		inputs, _ := filepath.Glob(filepath.Join(d, "temp*_input"))
		for _, in := range inputs {
			v, err := strconv.ParseFloat(readText(in), 64)
			if err != nil {
				continue
			}
			label := readText(strings.TrimSuffix(in, "_input") + "_label")
			if label == "" {
				label = strings.TrimSuffix(filepath.Base(in), "_input")
			}
			t := domain.Temperature{Sensor: sensor, Label: label, Celsius: v / 1000}
			t.Primary = (sensor == "coretemp" && strings.HasPrefix(label, "Package")) ||
				(sensor == "k10temp" && (label == "Tctl" || label == "Tdie")) ||
				(sensor == "cpu_thermal") // Raspberry Pi
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Primary && !out[b].Primary })
	return out
}

func readText(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

var notADisk = regexp.MustCompile(`^(loop|ram|zram|dm-|sr|md|nbd)`)

func disks(ctx context.Context) []domain.DiskHealth {
	out := []domain.DiskHealth{}
	entries, _ := os.ReadDir("/sys/block")
	smartctl, _ := exec.LookPath("smartctl")
	for _, e := range entries {
		name := e.Name()
		if notADisk.MatchString(name) {
			continue
		}
		d := domain.DiskHealth{Device: name}
		vendor := readText("/sys/block/" + name + "/device/vendor")
		if strings.HasPrefix(name, "vd") || strings.HasPrefix(name, "xvd") || vendor == "QEMU" {
			d.Virtual = true
			d.Model = "Virtual disk"
			out = append(out, d)
			continue
		}
		if smartctl == "" {
			d.Model = readText("/sys/block/" + name + "/device/model")
			out = append(out, d)
			continue
		}
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		raw, _ := exec.CommandContext(c, smartctl, "-j", "-H", "-A", "-i", "/dev/"+name).Output()
		cancel()
		var j struct {
			Model  string `json:"model_name"`
			Status *struct {
				Passed bool `json:"passed"`
			} `json:"smart_status"`
			Temp *struct {
				Current float64 `json:"current"`
			} `json:"temperature"`
			PowerOn *struct {
				Hours int64 `json:"hours"`
			} `json:"power_on_time"`
			NVMe *struct {
				Used int64 `json:"percentage_used"`
			} `json:"nvme_smart_health_information_log"`
			ATA *struct {
				Table []struct {
					ID  int `json:"id"`
					Raw struct {
						Value int64 `json:"value"`
					} `json:"raw"`
				} `json:"table"`
			} `json:"ata_smart_attributes"`
		}
		// smartctl's exit code is a warning bitmask: ignore it when there is JSON.
		if json.Unmarshal(raw, &j) != nil {
			continue
		}
		d.Model = j.Model
		if j.Status != nil {
			d.Healthy = &j.Status.Passed
		}
		if j.Temp != nil {
			d.Celsius = &j.Temp.Current
		}
		if j.PowerOn != nil {
			d.PowerOnHours = &j.PowerOn.Hours
		}
		if j.NVMe != nil {
			d.WearPct = &j.NVMe.Used
		}
		if j.ATA != nil {
			for _, a := range j.ATA.Table {
				if a.ID == 5 { // Reallocated_Sector_Ct
					v := a.Raw.Value
					d.Reallocated = &v
				}
			}
		}
		out = append(out, d)
	}
	return out
}

func battery() *domain.Battery {
	dirs, _ := filepath.Glob("/sys/class/power_supply/BAT*")
	if len(dirs) == 0 {
		return nil
	}
	c, err := strconv.Atoi(readText(filepath.Join(dirs[0], "capacity")))
	if err != nil {
		return nil
	}
	return &domain.Battery{ChargePct: c, Status: readText(filepath.Join(dirs[0], "status"))}
}

func failedUnits(ctx context.Context) []string {
	raw, err := exec.CommandContext(ctx, "systemctl", "list-units", "--state=failed", "--no-legend", "--plain").Output()
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if f := strings.Fields(l); len(f) > 0 {
			out = append(out, f[0])
		}
	}
	return out
}
