// Package brand holds the product name and everything derived from it.
// Renaming the project means editing this file.
package brand

// Version is set at build time (-ldflags -X); "dev" for local builds.
var Version = "dev"

const (
	Name  = "serverwarden"
	Agent = "serverwarden-agent"
	Title = "Control center"
	Repo  = "pedro00100/" + Name // on GitHub: releases and the update check

	ConfigDir       = "/etc/" + Name
	DataDir         = "/var/lib/" + Name
	AgentConfig     = ConfigDir + "/agent.json"
	AgentActionKeys = ConfigDir + "/action-keys.json"
	AgentStateDir   = "/var/lib/" + Agent

	CSRFHeader     = "X-Panel"
	SessionCookie  = "__Host-panel"
	CeremonyCookie = "__Host-panel-cer"
	TicketCookie   = "__Host-panel-2fa" // password accepted, code pending
)
