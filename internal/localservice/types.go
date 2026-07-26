package localservice

import (
	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

type Envelope struct {
	SchemaVersion string `json:"schema_version"`
	Data          any    `json:"data,omitempty"`
	Error         string `json:"error,omitempty"`
}

type SearchInput struct {
	Query  string `json:"query"`
	LensID string `json:"lens_id,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type Status struct {
	SchemaVersion string                `json:"schema_version"`
	ServiceState  string                `json:"service_state"`
	Memory        personalmemory.Status `json:"memory"`
	State         agentstate.Status     `json:"state"`
}

type DeleteResult struct {
	Deleted bool `json:"deleted"`
}
