package controlsettings

import "encoding/json"

const (
	DefaultProductBrainLandscapeLens = "Product Brain landscape: tools, resources, repositories, competitors, complements, and lessons relevant to building Product Brain."
	DefaultAIOrganizationLens        = "AI-dominant organization design: team structures, operating models, capabilities, incentives, and coordination patterns for transformational AI work."
	DefaultContentNarrativeLens      = "Content and narrative intelligence: trends, audience responses, recurring questions, objections, examples, and successful explanations that can inspire original content aligned with our identity, work, and Product Brain Chain. Identify what to build on, contradict, simplify, or explain better for less-technical audiences, and how Product Brain is meaningfully different."
	DefaultRoutingPolicy             = "Evaluate each source against every configured lens. Consider that one source may support multiple outcomes. In this bounded proof, preserve additional supported outcomes, including content opportunities, in the reviewed rationale rather than treating the selected role as exhaustive. Promote only evidence-backed, strategically relevant findings, entities, tensions, or references. Ground content opportunities in source evidence and connected strategic-authority evidence or decisions, then identify the observed audience need and an original angle. Treat engagement and comments as signals, not truth. Never copy or fabricate. Mindline does not generate or publish finished content in this proof. Hold incomplete or uncertain items. Send inaccessible and private sources to manual support."
)

func DefaultDraft() Draft {
	return Draft{
		ContextLenses: []string{
			DefaultProductBrainLandscapeLens,
			DefaultAIOrganizationLens,
			DefaultContentNarrativeLens,
		},
		RoutingPolicy: DefaultRoutingPolicy,
		DrainPolicy: DrainPolicy{
			MaximumNetworkRequests: 5000,
			MaximumWallTimeSeconds: 14400,
			MaximumCostMicrounits:  1000000,
			MaximumRetryAttempts:   2000,
			ManualSupportTolerance: 250,
		},
		AdapterDefaults: []AdapterDefault{{
			Slot: "source", AdapterKind: "slack_web_api",
			SchemaVersion: "mindline.source.slack-web-api-defaults/v1",
			Values:        json.RawMessage(`{"channel_id":"C0123456789"}`),
		}},
		ExpectedSourceIdentity:      nil,
		ExpectedDestinationIdentity: nil,
	}
}
