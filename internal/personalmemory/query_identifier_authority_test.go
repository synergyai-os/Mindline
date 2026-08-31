package personalmemory

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestQueryIdentifierAuthorityRecognitionMatrix(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		components [][]string
	}{
		{name: "generic paraphrase", query: "how should agents improve organization design"},
		{name: "closed AI concept", query: "AI and AI-heavy teams"},
		{name: "closed common query leads", query: "Could this Work and Use better governance"},
		{name: "single TitleCase", query: "Mindline lessons", components: [][]string{{"mindline"}}},
		{name: "non-leading TitleCase", query: "what did Mindline teach", components: [][]string{{"mindline"}}},
		{name: "multi TitleCase", query: "Alice Smith notes", components: [][]string{{"alice", "smith"}}},
		{name: "quoted phrase", query: `recall "local db" lessons`, components: [][]string{{"local", "db"}}},
		{name: "single quoted apostrophe", query: "recall 'O'Reilly' lessons", components: [][]string{{"o", "reilly"}}},
		{name: "handle", query: "posts by @dokterbob", components: [][]string{{"@dokterbob"}}},
		{name: "tag", query: "posts about #AgentOps", components: [][]string{{"#agentops"}}},
		{name: "camel case", query: "ProductBrain lessons", components: [][]string{{"productbrain"}}},
		{name: "acronym", query: "CLI lessons", components: [][]string{{"cli"}}},
		{name: "letters and digits", query: "ctx7 lessons", components: [][]string{{"ctx7"}}},
		{name: "repository", query: "dokterbob/localdb lessons", components: [][]string{{"dokterbob/localdb"}}},
		{name: "host", query: "github.com lessons", components: [][]string{{"github.com"}}},
		{name: "versioned product", query: "React 19 lessons", components: [][]string{{"react", "19"}}},
		{name: "hyphen version", query: "GPT-5 lessons", components: [][]string{{"gpt-5"}}},
		{name: "duplicate", query: "Mindline Mindline lessons", components: [][]string{{"mindline"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authority, err := BuildQueryIdentifierAuthority(test.query)
			if err != nil {
				t.Fatal(err)
			}
			got := make([][]string, 0, len(authority.Groups))
			for _, group := range authority.Groups {
				got = append(got, group.Components)
			}
			if len(got) != len(test.components) ||
				(len(got) > 0 && !reflect.DeepEqual(got, test.components)) {
				t.Fatalf("query=%q groups=%v want=%v", test.query, got, test.components)
			}
		})
	}
	if _, err := BuildQueryIdentifierAuthority(string([]byte{0xff})); err == nil {
		t.Fatal("invalid UTF-8 query was accepted")
	}
}

func TestQueryIdentifierAuthorityCanonicalizesUnicodeCaseAndPunctuation(t *testing.T) {
	nfc, err := BuildQueryIdentifierAuthority("Élodie Durand")
	if err != nil {
		t.Fatal(err)
	}
	nfd, err := BuildQueryIdentifierAuthority("E\u0301LODIE DURAND")
	if err != nil {
		t.Fatal(err)
	}
	if nfc.Fingerprint != nfd.Fingerprint || len(nfc.Groups) != 1 {
		t.Fatalf("Unicode/case identity diverged: nfc=%+v nfd=%+v", nfc, nfd)
	}
	punctuation, err := BuildQueryIdentifierAuthority("DokterBob／LocalDB")
	if err != nil {
		t.Fatal(err)
	}
	evidence := QueryIdentifierEvidenceForDocument(
		&punctuation, "dokterbob/localdb is the saved repository",
	)
	if !validQueryIdentifierEvidence(
		punctuation, "DOKTERBOB/LOCALDB is the saved repository", evidence,
	) {
		t.Fatalf("canonical punctuation/case evidence did not match: %+v", evidence)
	}
	apostrophe, err := BuildQueryIdentifierAuthority("O'Reilly")
	if err != nil {
		t.Fatal(err)
	}
	apostropheEvidence := QueryIdentifierEvidenceForDocument(&apostrophe, "O’REILLY reference")
	if !validQueryIdentifierEvidence(apostrophe, "O’REILLY reference", apostropheEvidence) {
		t.Fatalf("apostrophe variants diverged: %+v", apostropheEvidence)
	}
	mindline, err := BuildQueryIdentifierAuthority("Mindline")
	if err != nil {
		t.Fatal(err)
	}
	prefixOnly := QueryIdentifierEvidenceForDocument(&mindline, "Mindliner is unrelated")
	if validQueryIdentifierEvidence(mindline, "Mindliner is unrelated", prefixOnly) {
		t.Fatalf("substring was accepted as a whole identifier component: %+v", prefixOnly)
	}
}

type identifierAuthorityFixtureBackend struct {
	mutate          func(int, *RankedHit)
	mutateAuthority bool
}

func (identifierAuthorityFixtureBackend) MethodID() string {
	return "query-identifier-authority-fixture/v0.1"
}

func (identifierAuthorityFixtureBackend) CompactSemanticCalibrationID() string {
	return CompactSemanticCalibrationIdentity
}

func (backend identifierAuthorityFixtureBackend) Rank(
	request SearchRequest,
	documents []IndexDocument,
) ([]RankedHit, error) {
	if backend.mutateAuthority && request.QueryIdentifierAuthority != nil {
		request.QueryIdentifierAuthority.Groups = nil
	}
	hits := make([]RankedHit, 0, len(documents))
	for index, document := range documents {
		hit := RankedHit{
			DocumentID: document.DocumentID,
			Score:      float64(len(documents) - index),
			Components: map[string]float64{
				"semantic_cosine": 0.90 - float64(index)*0.05,
				"semantic_rank":   float64(index + 1),
				"semantic_top1":   0.90,
				"semantic_margin": 0.10,
			},
			IdentifierEvidence: QueryIdentifierEvidenceForDocument(
				request.QueryIdentifierAuthority, document.Text,
			),
		}
		if backend.mutate != nil {
			backend.mutate(index, &hit)
		}
		hits = append(hits, hit)
	}
	return hits, nil
}

func identifierAuthorityRepository(texts ...string) *compactRepository {
	records := make([]CaptureRecord, 0, len(texts))
	for index, text := range texts {
		id := "record-" + string(rune('a'+index))
		records = append(records, CaptureRecord{
			RecordID: id, SourceRef: "slack://fixture/" + id, RawText: text,
			ContentHash: strings.Repeat(string(rune('a'+index)), 64),
		})
	}
	return &compactRepository{library: Library{
		SchemaVersion: LibrarySchemaVersion,
		Revision:      1,
		Fingerprint:   strings.Repeat("f", 64),
		Records:       records,
	}}
}

func TestQueryIdentifierAuthorityFailsClosedForProviderEvidence(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*QueryIdentifierEvidence)
	}{
		{name: "omitted", mutate: func(value *QueryIdentifierEvidence) { *value = QueryIdentifierEvidence{} }},
		{name: "zero required", mutate: func(value *QueryIdentifierEvidence) { value.RequiredGroupCount = 0 }},
		{name: "wrong schema", mutate: func(value *QueryIdentifierEvidence) { value.SchemaVersion = "stale" }},
		{name: "wrong authority", mutate: func(value *QueryIdentifierEvidence) { value.AuthorityFingerprint = strings.Repeat("0", 64) }},
		{name: "fractional", mutate: func(value *QueryIdentifierEvidence) { value.MatchedGroupCount = 0.5 }},
		{name: "zero matched", mutate: func(value *QueryIdentifierEvidence) {
			value.MatchedGroupCount = 0
			value.MatchedGroupFingerprints = nil
		}},
		{name: "nonfinite", mutate: func(value *QueryIdentifierEvidence) { value.RequiredGroupCount = math.NaN() }},
		{name: "inconsistent list", mutate: func(value *QueryIdentifierEvidence) {
			value.MatchedGroupFingerprints = append(value.MatchedGroupFingerprints, strings.Repeat("1", 64))
			value.MatchedGroupCount++
		}},
	}
	for _, route := range []struct {
		name    string
		request SearchRequest
	}{
		{name: "unscoped", request: SearchRequest{Query: "Mindline", Limit: 3}},
		{name: "scoped", request: SearchRequest{Query: "Mindline", Limit: 3, ScopeID: "scope", AgentID: "agent"}},
	} {
		for _, mutation := range mutations {
			t.Run(route.name+"/"+mutation.name, func(t *testing.T) {
				backend := identifierAuthorityFixtureBackend{mutate: func(_ int, hit *RankedHit) {
					mutation.mutate(&hit.IdentifierEvidence)
				}}
				packet, err := NewRetriever(
					identifierAuthorityRepository("Mindline durable lessons"), backend,
				).SearchCompact(route.request)
				if err != nil {
					t.Fatal(err)
				}
				if packet.AnswerState != "abstained" || len(packet.Citations) != 0 {
					t.Fatalf("malformed provider evidence escaped: %+v", packet)
				}
			})
		}
	}
	packet, err := NewRetriever(
		identifierAuthorityRepository("durable semantic lessons"),
		identifierAuthorityFixtureBackend{},
	).SearchCompact(SearchRequest{Query: "durable lessons", Limit: 3})
	if err != nil || packet.AnswerState != "answered" {
		t.Fatalf("generic semantic recall changed: packet=%+v err=%v", packet, err)
	}
	packet, err = NewRetriever(
		identifierAuthorityRepository("durable semantic lessons"),
		identifierAuthorityFixtureBackend{mutate: func(_ int, hit *RankedHit) {
			hit.IdentifierEvidence = QueryIdentifierEvidence{}
		}},
	).SearchCompact(SearchRequest{Query: "durable lessons", Limit: 3})
	if err != nil || packet.AnswerState != "abstained" {
		t.Fatalf("omitted zero-group provider metadata did not fail closed: packet=%+v err=%v", packet, err)
	}
	packet, err = NewRetriever(
		identifierAuthorityRepository("Mindline durable lessons"),
		identifierAuthorityFixtureBackend{mutateAuthority: true},
	).SearchCompact(SearchRequest{Query: "Mindline", Limit: 3})
	if err != nil || packet.AnswerState != "abstained" {
		t.Fatalf("provider mutation changed core authority: packet=%+v err=%v", packet, err)
	}
}

func TestQueryIdentifierAuthorityEnforcesCitationAndPacketCoverage(t *testing.T) {
	tests := []struct {
		name              string
		query             string
		texts             []string
		expectedState     string
		expectedCitations int
	}{
		{
			name: "wrong extra removed per citation", query: "compare Mindline lessons",
			texts:         []string{"Mindline durable lessons", "unrelated durable lessons"},
			expectedState: "answered", expectedCitations: 1,
		},
		{
			name: "known and unknown comparison abstains", query: "compare Mindline PhantomKit",
			texts:         []string{"Mindline durable lessons", "unrelated durable lessons"},
			expectedState: "abstained", expectedCitations: 0,
		},
		{
			name: "known comparison covered across citations", query: "compare Mindline PhantomKit",
			texts:         []string{"Mindline durable lessons", "PhantomKit durable lessons"},
			expectedState: "answered", expectedCitations: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			packet, err := NewRetriever(
				identifierAuthorityRepository(test.texts...),
				identifierAuthorityFixtureBackend{},
			).SearchCompact(SearchRequest{
				Query: test.query, Limit: 3, ScopeID: "scope", AgentID: "agent",
			})
			if err != nil {
				t.Fatal(err)
			}
			if packet.AnswerState != test.expectedState ||
				len(packet.Citations) != test.expectedCitations {
				t.Fatalf("packet=%+v", packet)
			}
		})
	}
}

func TestQueryIdentifierAuthorityPreservesDegradedGenericRecallAndRejectsUnknownIdentity(t *testing.T) {
	repository := identifierAuthorityRepository("Mindline durable semantic lessons")
	retriever := NewLexicalRetriever(repository)
	matching, err := retriever.SearchCompact(SearchRequest{
		Query: "Mindline durable semantic lessons", Limit: 3,
	})
	if err != nil || matching.AnswerState != "answered" {
		t.Fatalf("matching identity did not survive degraded recall: packet=%+v err=%v", matching, err)
	}
	unknown, err := retriever.SearchCompact(SearchRequest{
		Query: "PhantomKit durable semantic lessons", Limit: 3,
	})
	if err != nil || unknown.AnswerState != "abstained" || len(unknown.Citations) != 0 {
		t.Fatalf("unknown identity was replaced by lexical neighbors: packet=%+v err=%v", unknown, err)
	}
	generic, err := retriever.SearchCompact(SearchRequest{
		Query: "durable semantic lessons", Limit: 3,
	})
	if err != nil || generic.AnswerState != "answered" {
		t.Fatalf("generic degraded recall changed: packet=%+v err=%v", generic, err)
	}
}
