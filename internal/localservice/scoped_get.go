package localservice

import (
	"errors"
	"net/http"

	"github.com/synergyai-os/Mindline/internal/agentcontract"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

func (server *Server) handleGetScoped(writer http.ResponseWriter, request *http.Request) {
	var input ScopedGetInput
	if err := decodeRequest(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, errors.New("invalid scoped hydration request"))
		return
	}
	authority, err := server.state.RequireScopedCandidate(
		request.Context(), input.RunID, input.ScopeID, input.LensID, input.AgentID, input.RecordID,
	)
	if err != nil {
		writeError(writer, http.StatusBadRequest, errors.New("scoped hydration rejected"))
		return
	}
	capture, err := personalmemory.NewLexicalRetriever(server.repository).GetScopedAtLibraryFingerprint(
		input.RecordID, authority.LibraryFingerprint, personalmemory.CompactSourceBinding{
			SchemaVersion: authority.SourceBinding.SchemaVersion,
			SourceKind:    authority.SourceBinding.SourceKind,
			SourceID:      authority.SourceBinding.SourceID,
			ContentHash:   authority.SourceBinding.ContentHash,
		},
	)
	if err != nil {
		writeError(writer, http.StatusNotFound, errors.New("scoped hydration rejected"))
		return
	}
	capture.RunID, capture.ScopeID, capture.LensID = input.RunID, input.ScopeID, input.LensID
	capture.AgentID = input.AgentID
	capture.RouteClass = agentcontract.GovernedRouteClass
	capture.AgentRecallApproved = true
	writeJSON(writer, http.StatusOK, capture)
}
