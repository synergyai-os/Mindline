package resourcefetch

import "strings"

// Profile selects required public context without changing canonical evidence
// schema. It is an extraction policy, not a destination adapter.
type Profile string

const (
	ProfileGeneric  Profile = "generic"
	ProfileHTML     Profile = "html_article"
	ProfileGitHub   Profile = "github"
	ProfileYouTube  Profile = "youtube"
	ProfileSpotify  Profile = "spotify"
	ProfileLinkedIn Profile = "linkedin"
)

func ProfileForHost(host string) Profile {
	host = strings.ToLower(host)
	switch {
	case host == "github.com" || strings.HasSuffix(host, ".github.com") || host == "raw.githubusercontent.com":
		return ProfileGitHub
	case host == "youtube.com" || strings.HasSuffix(host, ".youtube.com") || host == "youtu.be":
		return ProfileYouTube
	case host == "spotify.com" || strings.HasSuffix(host, ".spotify.com"):
		return ProfileSpotify
	case host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com"):
		return ProfileLinkedIn
	default:
		return ProfileHTML
	}
}

func profileMissingness(profile Profile, result Result) []string {
	missing := []string{}
	switch profile {
	case ProfileGitHub:
		if result.Text == "" {
			missing = append(missing, "github_readme_unavailable")
		}
	case ProfileYouTube:
		missing = append(missing, "transcript_not_publicly_accessible")
	case ProfileSpotify:
		if result.Text == "" {
			missing = append(missing, "media_body_not_publicly_accessible")
		}
	case ProfileLinkedIn:
		missing = append(missing, "comments_not_publicly_accessible")
	}
	return missing
}
