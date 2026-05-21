package productbrain

import (
	"fmt"
	"strings"
)

type ResolveInput struct {
	RunID        string
	ReviewItemID string
	SafeTitle    string
	SafeContext  string
	Reason       string
	Intent       Intent
}

func Resolve(input ResolveInput, profile Profile) Proposal {
	if input.Intent == "" {
		input.Intent = inferIntent(input)
	}
	if input.Intent == IntentNoProductBrainWrite {
		return blockedProposal(input, ProposalStatusSkipped, "no_product_brain_write", "Item does not require Product Brain state.")
	}
	mapping, mappingCount := findMapping(profile, input.Intent)
	if mappingCount > 1 {
		return blockedProposal(input, ProposalStatusBlocked, "ambiguous_intent_mapping", fmt.Sprintf("Profile defines multiple mappings for intent %s.", input.Intent))
	}
	ok := mappingCount == 1
	if !ok {
		return blockedProposal(input, ProposalStatusBlocked, "missing_intent_mapping", fmt.Sprintf("No profile mapping for intent %s.", input.Intent))
	}
	collection, ok := findCollection(profile, mapping.CollectionSlug)
	if !ok {
		return blockedProposal(input, ProposalStatusBlocked, "missing_collection", fmt.Sprintf("Collection %s is not present in profile.", mapping.CollectionSlug))
	}
	if collection.PlatformOnly {
		return blockedProposal(input, ProposalStatusBlocked, "platform_only_collection", fmt.Sprintf("Collection %s is platform-only.", collection.Slug))
	}
	data := map[string]string{}
	for role, fieldKey := range mapping.FieldMap {
		value := valueForRole(role, input)
		if value != "" {
			data[fieldKey] = value
		}
	}
	for _, field := range collection.Fields {
		if !field.Required {
			continue
		}
		if strings.TrimSpace(data[field.Key]) == "" {
			return blockedProposal(input, ProposalStatusBlocked, "missing_required_field", fmt.Sprintf("Required field %s cannot be populated safely.", field.Key))
		}
	}
	workflowStatus := collection.DefaultWorkflowStatus
	if workflowStatus == "" && len(collection.ValidWorkflowStatuses) > 0 {
		workflowStatus = collection.ValidWorkflowStatuses[0]
	}
	return NewProposal(ProposalInput{
		RunID:                input.RunID,
		SourceReviewItemID:   input.ReviewItemID,
		Intent:               input.Intent,
		Status:               ProposalStatusReady,
		TargetCollectionSlug: collection.Slug,
		EntryName:            input.SafeTitle,
		WorkflowStatus:       workflowStatus,
		Data:                 data,
	})
}

func inferIntent(input ResolveInput) Intent {
	text := strings.ToLower(input.SafeTitle + " " + input.SafeContext + " " + input.Reason)
	switch {
	case strings.Contains(text, "decision") || strings.Contains(text, "choose") || strings.Contains(text, "choice"):
		return IntentDurableDecision
	case strings.Contains(text, "insight") || strings.Contains(text, "learning") || strings.Contains(text, "pattern"):
		return IntentReusableInsight
	case strings.Contains(text, "unknown") || strings.Contains(text, "blocked"):
		return IntentOpenTension
	default:
		return IntentNoProductBrainWrite
	}
}

func findMapping(profile Profile, intent Intent) (IntentMapping, int) {
	var found IntentMapping
	count := 0
	for _, mapping := range profile.IntentMappings {
		if mapping.Intent == intent {
			if count == 0 {
				found = mapping
			}
			count++
		}
	}
	return found, count
}

func findCollection(profile Profile, slug string) (Collection, bool) {
	for _, collection := range profile.Collections {
		if collection.Slug == slug {
			return collection, true
		}
	}
	return Collection{}, false
}

func valueForRole(role string, input ResolveInput) string {
	switch role {
	case "rationale", "summary":
		return safeText(input.SafeContext)
	case "title", "name":
		return safeText(input.SafeTitle)
	default:
		return safeText(input.SafeContext)
	}
}

func blockedProposal(input ResolveInput, status ProposalStatus, code string, message string) Proposal {
	return NewProposal(ProposalInput{
		RunID:              input.RunID,
		SourceReviewItemID: input.ReviewItemID,
		Intent:             input.Intent,
		Status:             status,
		EntryName:          input.SafeTitle,
		Blockers:           []Blocker{{Code: code, Message: message}},
	})
}
