package daemon

type CommandRequest struct {
	Command         string `json:"command"`
	ManifestPath    string `json:"manifestPath,omitempty"`
	Name            string `json:"name,omitempty"`
	AllowDisruptive bool   `json:"allowDisruptive,omitempty"`
	NoCache         bool   `json:"noCache,omitempty"`
	Force           bool   `json:"force,omitempty"`
	OutputJSON      bool   `json:"outputJson,omitempty"`
}

type CommandResponse struct {
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}
