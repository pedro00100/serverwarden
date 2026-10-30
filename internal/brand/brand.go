// Package brand holds the product name and everything derived from it.
// Renaming the project means editing this file.
package brand

const (
	Name  = "serverwarden"
	Agent = "serverwarden-agent"
	Title = "Control center"

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
