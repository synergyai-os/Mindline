package localservice

import (
	"errors"
	"net/http"

	"github.com/synergyai-os/Mindline/internal/agentstate"
)

func (server *Server) handleBindProjectConnection(writer http.ResponseWriter, request *http.Request) {
	var input ProjectConnectionInput
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	connection, err := server.state.BindProjectConnection(request.Context(), input.Digest,
		agentstate.ScopedContext{ScopeID: input.ScopeID, LensID: input.LensID, AgentID: input.AgentID})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, agentstate.ErrProjectConnectionOutcomeUnknown) {
			status = http.StatusServiceUnavailable
		} else if errors.Is(err, agentstate.ErrProjectConnectionConflict) ||
			errors.Is(err, agentstate.ErrProjectConnectionArchived) {
			status = http.StatusConflict
		}
		writeError(writer, status, err)
		return
	}
	writeJSON(writer, http.StatusOK, ProjectConnectionReceipt{
		SchemaVersion: "mindline-project-connection-receipt/v0.1",
		State:         agentstate.StatusActive, Replayed: connection.Replayed,
	})
}

func (server *Server) handleResolveProjectConnection(writer http.ResponseWriter, request *http.Request) {
	var input ProjectConnectionDigestInput
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	_, scope, lens, actor, err := server.state.ResolveProjectConnection(request.Context(), input.Digest)
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, agentstate.ErrProjectConnectionOutcomeUnknown) {
			status = http.StatusServiceUnavailable
		}
		writeError(writer, status, err)
		return
	}
	writeJSON(writer, http.StatusOK, ProjectConnectionResolution{
		SchemaVersion: "mindline-project-connection-resolution/v0.1", State: "ready",
		ScopeID: scope.ID, ScopeName: scope.Name, LensID: lens.ID, LensName: lens.Name,
		AgentID: actor.ID, AgentName: actor.Name,
	})
}

func (server *Server) handleArchiveProjectConnection(writer http.ResponseWriter, request *http.Request) {
	var input ProjectConnectionDigestInput
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	connection, err := server.state.ArchiveProjectConnection(request.Context(), input.Digest)
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, agentstate.ErrProjectConnectionOutcomeUnknown) {
			status = http.StatusServiceUnavailable
		}
		writeError(writer, status, err)
		return
	}
	writeJSON(writer, http.StatusOK, ProjectConnectionReceipt{
		SchemaVersion: "mindline-project-connection-receipt/v0.1",
		State:         agentstate.StatusArchived, Replayed: connection.Replayed,
	})
}
