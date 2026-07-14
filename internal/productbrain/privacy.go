package productbrain

import (
	"encoding/json"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type PrivacyFinding struct {
	Category string `json:"category"`
	JSONPath string `json:"json_path"`
}

var slackIDPattern = regexp.MustCompile(`\b[CDUGW][A-Z0-9]{8,}\b`)
var secretPattern = regexp.MustCompile(`(?i)(pb_sk_|xox[baprs]-|sk_live_|sk-proj-|bearer\s+|api[_-]?key\s*[=:]|authorization)`)

func ScanPublicArtifact(value any, exactSecret string) []PrivacyFinding {
	data, _ := json.Marshal(value)
	var raw any
	_ = json.Unmarshal(data, &raw)
	var findings []PrivacyFinding
	scanPublicValue(raw, "$", exactSecret, &findings)
	return findings
}
func scanPublicValue(value any, path string, exactSecret string, findings *[]PrivacyFinding) {
	switch x := value.(type) {
	case map[string]any:
		for key, v := range x {
			lower := strings.ToLower(key)
			next := path + "." + key
			if forbiddenField(lower) {
				*findings = append(*findings, PrivacyFinding{Category: "forbidden_field", JSONPath: next})
			}
			scanPublicValue(v, next, exactSecret, findings)
		}
	case []any:
		for i, v := range x {
			scanPublicValue(v, path+"["+strconv.Itoa(i)+"]", exactSecret, findings)
		}
	case string:
		if exactSecret != "" && strings.Contains(x, exactSecret) {
			*findings = append(*findings, PrivacyFinding{Category: "runtime_secret", JSONPath: path})
		}
		lower := strings.ToLower(x)
		switch {
		case secretPattern.MatchString(x):
			*findings = append(*findings, PrivacyFinding{Category: "secret_pattern", JSONPath: path})
		case slackIDPattern.MatchString(x) || strings.Contains(lower, "slack.com/archives/") || strings.HasPrefix(lower, "slack://") || strings.HasPrefix(lower, "adapter-local://slack") || strings.HasPrefix(lower, "adapter-local://"):
			*findings = append(*findings, PrivacyFinding{Category: "private_source_identity", JSONPath: path})
		case strings.Contains(lower, "/users/") || strings.Contains(lower, "/private/") || strings.HasPrefix(lower, "file://"):
			*findings = append(*findings, PrivacyFinding{Category: "local_path", JSONPath: path})
		case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
			if !publicURL(x) {
				*findings = append(*findings, PrivacyFinding{Category: "private_url", JSONPath: path})
			}
		}
	}
}
func forbiddenField(key string) bool {
	for _, term := range []string{"channel_id", "conversation_id", "slack_ts", "permalink", "raw_text", "message_ts", "thread_ts", "api_key", "authorization", "cookie", "local_path", "workspace_path"} {
		if key == term {
			return true
		}
	}
	return false
}
func publicURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast()) {
		return false
	}
	for key := range u.Query() {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "signature") || strings.Contains(lower, "expires") || lower == "x-amz-credential" {
			return false
		}
	}
	return true
}
