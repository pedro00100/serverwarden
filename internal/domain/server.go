// Package domain is the core model: servers, what their agents report and how
// health is judged. It has no external dependencies.
package domain

import "time"

type Server struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Short    string `json:"short"`
	Role     string `json:"role"`
	Location string `json:"location"`
	// Layer places the server in the topology: "edge" faces the internet, "internal" does not.
	Layer string `json:"layer"`
}

// Snapshot is the state of a server as reported by its agent.
type Snapshot struct {
	Hostname       string          `json:"hostname"`
	OS             string          `json:"os"`
	Kernel         string          `json:"kernel"`
	UptimeSec      int64           `json:"uptime_sec"`
	Cores          int             `json:"cores"`
	Load           [3]float64      `json:"load"`
	CPU            float64         `json:"cpu_pct"`
	CPUSteal       float64         `json:"cpu_steal_pct"`
	Memory         Usage           `json:"memory"`
	Swap           Usage           `json:"swap"`
	Disk           Usage           `json:"disk"`
	Temperatures   []Temperature   `json:"temperatures"`
	Disks          []DiskHealth    `json:"disks"`
	Battery        *Battery        `json:"battery,omitempty"`
	Containers     []Container     `json:"containers"`
	FailedUnits    []string        `json:"failed_units"`
	RebootRequired bool            `json:"reboot_required"`
	Tools          map[string]bool `json:"tools"`
	Actions        []Action        `json:"actions"`
	PanelCommands  bool            `json:"panel_commands"` // accepts the commands defined in the panel
	ImageExport    bool            `json:"image_export"`   // lets the panel export local images to scan them
	ConfigError    string          `json:"config_error,omitempty"`
	TakenAt        time.Time       `json:"taken_at"`
}

// Usage is an amount used out of a total, in bytes.
type Usage struct {
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
}

func (u Usage) Pct() float64 {
	if u.Total == 0 {
		return 0
	}
	return float64(u.Used) * 100 / float64(u.Total)
}

type Temperature struct {
	Sensor  string  `json:"sensor"`
	Label   string  `json:"label"`
	Celsius float64 `json:"celsius"`
	// Primary marks the reading that represents the CPU.
	Primary bool `json:"primary"`
}

// DiskHealth summarizes S.M.A.R.T. data. Nil fields are not reported by the disk.
type DiskHealth struct {
	Device       string   `json:"device"`
	Model        string   `json:"model"`
	Healthy      *bool    `json:"healthy"`
	Celsius      *float64 `json:"celsius"`
	Reallocated  *int64   `json:"reallocated"`
	PowerOnHours *int64   `json:"power_on_hours"`
	WearPct      *int64   `json:"wear_pct"`
	Virtual      bool     `json:"virtual"`
}

type Battery struct {
	ChargePct int    `json:"charge_pct"`
	Status    string `json:"status"`
}

type Container struct {
	Name   string       `json:"name"`
	Image  string       `json:"image"`
	State  string       `json:"state"`  // running, exited, restarting…
	Health string       `json:"health"` // healthy, unhealthy, starting or empty
	Status string       `json:"status"`
	Vault  *VaultStatus `json:"vault,omitempty"`
}

// Image is the image of a running container, as the vulnerability scanner
// needs it: Digests (repo@sha256:…) exist when it came from a registry.
type Image struct {
	Ref      string   `json:"ref"`
	ID       string   `json:"id"`
	Digests  []string `json:"digests"`
	Platform string   `json:"platform"` // os/arch[/variant]
}

type VaultStatus struct {
	Initialized bool   `json:"initialized"`
	Sealed      bool   `json:"sealed"`
	Version     string `json:"version"`
	Progress    int    `json:"progress"` // unseal keys entered so far
	Threshold   int    `json:"threshold"`
}
