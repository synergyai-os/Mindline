package personalmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const QueryIdentifierAuthoritySchemaVersion = "query_identifier_authority/v1"

// QueryIdentifierAuthority is the immutable query-only identity contract for
// one compact retrieval. Groups contain only normalized in-memory components;
// raw identifiers never enter packets, receipts, logs, or telemetry.
type QueryIdentifierAuthority struct {
	SchemaVersion string
	Fingerprint   string
	Groups        []QueryIdentifierGroup
}

type QueryIdentifierGroup struct {
	Fingerprint string
	Kind        string
	Components  []string
}

// QueryIdentifierEvidence is a backend attestation. Core compact retrieval
// treats it as untrusted and compares it with an independent recomputation.
// Floating counts deliberately make omitted, non-finite, and fractional
// provider metadata representable and therefore explicitly rejectable.
type QueryIdentifierEvidence struct {
	SchemaVersion            string
	AuthorityFingerprint     string
	RequiredGroupCount       float64
	MatchedGroupCount        float64
	MatchedGroupFingerprints []string
}

var queryIdentifierFold = cases.Fold()

var queryIdentifierClosedConcepts = map[string]bool{
	"a": true, "ai": true, "ai-heavy": true, "ai-native": true,
	"an": true, "and": true, "answer": true, "are": true, "as": true, "at": true,
	"agent": true, "agents": true, "assess": true, "be": true, "by": true,
	"can": true, "check": true, "compare": true, "consider": true, "could": true,
	"create": true, "decision": true, "describe": true, "did": true,
	"discuss": true, "do": true, "does": true, "draft": true,
	"evaluate": true, "explain": true, "explore": true, "find": true,
	"for": true, "from": true, "get": true, "gets": true, "give": true,
	"got": true, "governance": true, "help": true, "highlight": true,
	"how": true, "i": true, "idea": true, "ideas": true, "identify": true,
	"if": true, "in": true, "is": true, "it": true, "know": true,
	"list": true, "look": true, "make": true, "makes": true, "may": true,
	"me": true, "my": true, "name": true, "need": true, "of": true,
	"on": true, "or": true, "organization": true, "outline": true,
	"please": true, "product": true, "project": true, "provide": true,
	"recall": true, "recommend": true, "remember": true, "report": true,
	"retrieve": true, "review": true, "search": true, "share": true,
	"should": true, "show": true, "state": true, "suggest": true,
	"summarize": true, "surface": true, "team": true, "tell": true,
	"that": true, "the": true, "these": true, "thing": true, "things": true,
	"this": true, "to": true, "use": true, "used": true, "using": true,
	"versus": true, "vs": true, "want": true, "was": true, "way": true,
	"ways": true, "were": true, "what": true, "when": true, "where": true,
	"which": true, "who": true, "why": true, "will": true, "with": true,
	"work": true, "working": true, "works": true, "would": true, "write": true,
}

type queryIdentifierLexeme struct {
	raw          string
	quoted       bool
	letters      int
	upper, lower int
}

// BuildQueryIdentifierAuthority creates the sole compact query identifier
// contract. It is deterministic, query-only, bounded by the existing request
// budget, and contains no scope, lens, agent, provider, or feedback state.
func BuildQueryIdentifierAuthority(query string) (QueryIdentifierAuthority, error) {
	canonical, err := canonicalQueryIdentifierText(query, false)
	if err != nil {
		return QueryIdentifierAuthority{}, err
	}
	groups := queryIdentifierGroups(canonical)
	identity := []string{QueryIdentifierAuthoritySchemaVersion}
	for _, group := range groups {
		identity = append(identity, group.Fingerprint)
	}
	foldedCanonical, err := canonicalQueryIdentifierText(canonical, true)
	if err != nil {
		return QueryIdentifierAuthority{}, err
	}
	querySum := sha256.Sum256([]byte(foldedCanonical))
	identity = append(identity, hex.EncodeToString(querySum[:]))
	sum := sha256.Sum256([]byte(strings.Join(identity, "\n")))
	return QueryIdentifierAuthority{
		SchemaVersion: QueryIdentifierAuthoritySchemaVersion,
		Fingerprint:   hex.EncodeToString(sum[:]),
		Groups:        groups,
	}, nil
}

func cloneQueryIdentifierAuthority(authority QueryIdentifierAuthority) QueryIdentifierAuthority {
	cloned := authority
	cloned.Groups = make([]QueryIdentifierGroup, len(authority.Groups))
	for index, group := range authority.Groups {
		cloned.Groups[index] = group
		cloned.Groups[index].Components = append([]string(nil), group.Components...)
	}
	return cloned
}

func canonicalQueryIdentifierText(value string, fold bool) (string, error) {
	if !utf8.ValidString(value) {
		return "", errors.New("query identifier authority requires valid UTF-8")
	}
	value = norm.NFKC.String(value)
	value = strings.Map(func(character rune) rune {
		switch character {
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
			return '-'
		case '\u2018', '\u2019', '\u201b', '\u02bc', '\uff07':
			return '\''
		case '\uff0f':
			return '/'
		case '\uff0e':
			return '.'
		case '\uff1a':
			return ':'
		}
		return character
	}, value)
	if fold {
		value = queryIdentifierFold.String(value)
		value = norm.NFKC.String(value)
	}
	return value, nil
}

func queryIdentifierGroups(query string) []QueryIdentifierGroup {
	lexemes := scanQueryIdentifierLexemes(query)
	groups := []QueryIdentifierGroup{}
	seen := map[string]bool{}
	add := func(kind string, components []string) {
		components = uniqueInOrder(components)
		if len(components) == 0 {
			return
		}
		key := strings.Join(components, "\x00")
		if seen[key] {
			return
		}
		seen[key] = true
		sum := sha256.Sum256([]byte(QueryIdentifierAuthoritySchemaVersion + "\n" + key))
		groups = append(groups, QueryIdentifierGroup{
			Fingerprint: hex.EncodeToString(sum[:]), Kind: kind,
			Components: append([]string(nil), components...),
		})
	}
	used := make([]bool, len(lexemes))
	for index, lexeme := range lexemes {
		if lexeme.quoted {
			add("quoted", normalizedIdentifierWordComponents(lexeme.raw))
			used[index] = true
		}
	}
	for index := 0; index+1 < len(lexemes); index++ {
		if used[index] || used[index+1] || !identifierProductWord(lexemes[index]) ||
			!identifierVersionNumber(lexemes[index+1].raw) {
			continue
		}
		add("versioned", []string{
			normalizedIdentifierComponent(lexemes[index].raw),
			normalizedIdentifierComponent(lexemes[index+1].raw),
		})
		used[index], used[index+1] = true, true
	}
	for index := 0; index < len(lexemes); {
		if used[index] || identifierClosedConcept(lexemes[index].raw) ||
			lexemes[index].upper < 2 || lexemes[index].upper != lexemes[index].letters {
			index++
			continue
		}
		end := index
		components := []string{}
		for end < len(lexemes) && !used[end] &&
			!identifierClosedConcept(lexemes[end].raw) &&
			lexemes[end].upper >= 2 && lexemes[end].upper == lexemes[end].letters {
			components = append(components, normalizedIdentifierComponent(lexemes[end].raw))
			end++
		}
		add("acronym", components)
		for position := index; position < end; position++ {
			used[position] = true
		}
		index = end
	}
	for index, lexeme := range lexemes {
		if used[index] || identifierClosedConcept(lexeme.raw) {
			continue
		}
		kind := ""
		switch {
		case strings.HasPrefix(lexeme.raw, "@") || strings.HasPrefix(lexeme.raw, "#"):
			kind = "handle_or_tag"
		case identifierHasInternalSyntax(lexeme.raw):
			kind = "compound"
		case identifierMixedCase(lexeme.raw):
			kind = "mixed_case"
		case identifierLettersAndDigits(lexeme.raw):
			kind = "alphanumeric"
		}
		if kind != "" {
			add(kind, []string{normalizedIdentifierComponent(lexeme.raw)})
			used[index] = true
		}
	}
	for index := 0; index < len(lexemes); {
		if used[index] || !identifierTitleCase(lexemes[index]) ||
			identifierClosedConcept(lexemes[index].raw) {
			index++
			continue
		}
		end := index
		components := []string{}
		for end < len(lexemes) && !used[end] && identifierTitleCase(lexemes[end]) &&
			!identifierClosedConcept(lexemes[end].raw) {
			components = append(components, normalizedIdentifierComponent(lexemes[end].raw))
			end++
		}
		add("title_case", components)
		for position := index; position < end; position++ {
			used[position] = true
		}
		index = end
	}
	return groups
}

func scanQueryIdentifierLexemes(value string) []queryIdentifierLexeme {
	runes := []rune(value)
	lexemes := []queryIdentifierLexeme{}
	for index := 0; index < len(runes); {
		if runes[index] == '"' || runes[index] == '`' || runes[index] == '\'' {
			quote := runes[index]
			end := index + 1
			for end < len(runes) {
				if runes[end] != quote {
					end++
					continue
				}
				if quote == '\'' && end > index+1 && end+1 < len(runes) &&
					unicode.IsLetter(runes[end-1]) && unicode.IsLetter(runes[end+1]) {
					end++
					continue
				}
				break
			}
			if end < len(runes) && end > index+1 {
				lexemes = append(lexemes, newQueryIdentifierLexeme(string(runes[index+1:end]), true))
				index = end + 1
				continue
			}
		}
		if !queryIdentifierRune(runes[index]) {
			index++
			continue
		}
		end := index + 1
		for end < len(runes) && queryIdentifierRune(runes[end]) {
			end++
		}
		raw := trimQueryIdentifierToken(string(runes[index:end]))
		if raw != "" {
			lexemes = append(lexemes, newQueryIdentifierLexeme(raw, false))
		}
		index = end
	}
	return lexemes
}

func queryIdentifierRune(character rune) bool {
	return unicode.IsLetter(character) || unicode.IsDigit(character) ||
		strings.ContainsRune("@#._-/:'", character)
}

func trimQueryIdentifierToken(value string) string {
	runes := []rune(value)
	for len(runes) > 0 && strings.ContainsRune("._-/:'", runes[0]) {
		runes = runes[1:]
	}
	for len(runes) > 0 && strings.ContainsRune("._-/:'", runes[len(runes)-1]) {
		runes = runes[:len(runes)-1]
	}
	if len(runes) == 1 && (runes[0] == '@' || runes[0] == '#') {
		return ""
	}
	return string(runes)
}

func newQueryIdentifierLexeme(raw string, quoted bool) queryIdentifierLexeme {
	lexeme := queryIdentifierLexeme{raw: strings.TrimSpace(raw), quoted: quoted}
	for _, character := range raw {
		if unicode.IsLetter(character) {
			lexeme.letters++
		}
		if unicode.IsUpper(character) {
			lexeme.upper++
		} else if unicode.IsLower(character) {
			lexeme.lower++
		}
	}
	return lexeme
}

func normalizedIdentifierComponent(value string) string {
	value, _ = canonicalQueryIdentifierText(value, true)
	return strings.TrimSpace(value)
}

func normalizedIdentifierWordComponents(value string) []string {
	value, _ = canonicalQueryIdentifierText(value, true)
	components := []string{}
	current := []rune{}
	flush := func() {
		if len(current) > 0 {
			components = append(components, string(current))
			current = nil
		}
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			current = append(current, character)
		} else {
			flush()
		}
	}
	flush()
	return components
}

func identifierHasInternalSyntax(value string) bool {
	for _, character := range strings.TrimLeft(value, "@#") {
		if strings.ContainsRune("._-/:'", character) {
			return true
		}
	}
	return false
}

func identifierMixedCase(value string) bool {
	seenLower := false
	for _, character := range value {
		if unicode.IsLower(character) {
			seenLower = true
		} else if unicode.IsUpper(character) && seenLower {
			return true
		}
	}
	return false
}

func identifierLettersAndDigits(value string) bool {
	letter, digit := false, false
	for _, character := range value {
		letter = letter || unicode.IsLetter(character)
		digit = digit || unicode.IsDigit(character)
	}
	return letter && digit
}

func identifierTitleCase(lexeme queryIdentifierLexeme) bool {
	runes := []rune(lexeme.raw)
	return len(runes) > 0 && unicode.IsUpper(runes[0]) && lexeme.upper == 1 && lexeme.lower > 0 &&
		!identifierHasInternalSyntax(lexeme.raw)
}

func identifierProductWord(lexeme queryIdentifierLexeme) bool {
	return !identifierClosedConcept(lexeme.raw) &&
		(identifierTitleCase(lexeme) || identifierMixedCase(lexeme.raw) || lexeme.upper >= 2)
}

func identifierVersionNumber(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if !unicode.IsDigit(character) {
			return false
		}
	}
	return true
}

func identifierClosedConcept(value string) bool {
	return queryIdentifierClosedConcepts[normalizedIdentifierComponent(value)]
}

// QueryIdentifierEvidenceForDocument is the canonical backend-side attestation
// helper. Core compact retrieval recomputes the same evidence and compares the
// complete typed value before it authorizes any citation.
func QueryIdentifierEvidenceForDocument(
	authority *QueryIdentifierAuthority,
	documentText string,
) QueryIdentifierEvidence {
	if authority == nil || authority.SchemaVersion != QueryIdentifierAuthoritySchemaVersion ||
		strings.TrimSpace(authority.Fingerprint) == "" {
		return QueryIdentifierEvidence{}
	}
	components, err := queryIdentifierDocumentComponents(documentText)
	if err != nil {
		return QueryIdentifierEvidence{}
	}
	matched := []string{}
	for _, group := range authority.Groups {
		complete := len(group.Components) > 0
		for _, component := range group.Components {
			if !components[component] {
				complete = false
				break
			}
		}
		if complete {
			matched = append(matched, group.Fingerprint)
		}
	}
	sort.Strings(matched)
	return QueryIdentifierEvidence{
		SchemaVersion:            QueryIdentifierAuthoritySchemaVersion,
		AuthorityFingerprint:     authority.Fingerprint,
		RequiredGroupCount:       float64(len(authority.Groups)),
		MatchedGroupCount:        float64(len(matched)),
		MatchedGroupFingerprints: matched,
	}
}

func queryIdentifierDocumentComponents(value string) (map[string]bool, error) {
	value, err := canonicalQueryIdentifierText(value, false)
	if err != nil {
		return nil, err
	}
	components := map[string]bool{}
	for _, lexeme := range scanQueryIdentifierLexemes(value) {
		if component := normalizedIdentifierComponent(lexeme.raw); component != "" {
			components[component] = true
		}
		for _, component := range normalizedIdentifierWordComponents(lexeme.raw) {
			components[component] = true
		}
	}
	return components, nil
}

func validQueryIdentifierEvidence(
	authority QueryIdentifierAuthority,
	documentText string,
	actual QueryIdentifierEvidence,
) bool {
	if authority.SchemaVersion != QueryIdentifierAuthoritySchemaVersion ||
		strings.TrimSpace(authority.Fingerprint) == "" ||
		actual.SchemaVersion != QueryIdentifierAuthoritySchemaVersion ||
		actual.AuthorityFingerprint != authority.Fingerprint ||
		math.IsNaN(actual.RequiredGroupCount) || math.IsInf(actual.RequiredGroupCount, 0) ||
		math.IsNaN(actual.MatchedGroupCount) || math.IsInf(actual.MatchedGroupCount, 0) ||
		actual.RequiredGroupCount != float64(len(authority.Groups)) ||
		actual.MatchedGroupCount != float64(len(actual.MatchedGroupFingerprints)) {
		return false
	}
	expected := QueryIdentifierEvidenceForDocument(&authority, documentText)
	if expected.SchemaVersion == "" || expected.AuthorityFingerprint != actual.AuthorityFingerprint ||
		expected.RequiredGroupCount != actual.RequiredGroupCount ||
		expected.MatchedGroupCount != actual.MatchedGroupCount ||
		len(expected.MatchedGroupFingerprints) != len(actual.MatchedGroupFingerprints) {
		return false
	}
	for index := range expected.MatchedGroupFingerprints {
		if expected.MatchedGroupFingerprints[index] != actual.MatchedGroupFingerprints[index] ||
			(index > 0 && actual.MatchedGroupFingerprints[index-1] >= actual.MatchedGroupFingerprints[index]) {
			return false
		}
	}
	return len(authority.Groups) == 0 || len(actual.MatchedGroupFingerprints) > 0
}

func queryIdentifierPacketComplete(authority QueryIdentifierAuthority, hits []RankedHit) bool {
	if len(authority.Groups) == 0 {
		return true
	}
	covered := map[string]bool{}
	for _, hit := range hits {
		for _, fingerprint := range hit.IdentifierEvidence.MatchedGroupFingerprints {
			covered[fingerprint] = true
		}
	}
	for _, group := range authority.Groups {
		if !covered[group.Fingerprint] {
			return false
		}
	}
	return len(hits) > 0
}
