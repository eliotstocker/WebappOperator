package server

import (
	"embed"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	webappv1alpha1 "github.com/eliotstocker/webapp-operator/api/v1alpha1"
)

//go:embed *.json
var embeddedBotFiles embed.FS

var defaultBotTags = map[string]struct{}{
	"search-engine":  {},
	"social-preview": {},
	"ai-crawler":     {},
	"seo":            {},
}

type crawlerEntry struct {
	Pattern string   `json:"pattern"`
	Tags    []string `json:"tags"`
}

// defaultBotRegex is compiled at startup from crawler-user-agents.json when fetched at build time.
// If the file was not fetched (e.g. direct `go test`), it remains nil and defaultBotUserAgents is used.
var defaultBotRegex = initDefaultBotRegex()

func initDefaultBotRegex() *regexp.Regexp {
	data, err := embeddedBotFiles.ReadFile("crawler-user-agents.json")
	if err != nil {
		return nil
	}
	return loadCrawlerRegexFromJSON(data)
}

// loadCrawlerRegexFromJSON parses a monperrus/crawler-user-agents JSON payload, filters entries
// to search/social/AI/SEO crawlers, and compiles RE2-compatible patterns into one regex.
func loadCrawlerRegexFromJSON(data []byte) *regexp.Regexp {
	var entries []crawlerEntry
	if err := json.Unmarshal(data, &entries); err != nil || len(entries) == 0 {
		return nil
	}

	var valid []string
	for _, entry := range entries {
		if entry.Pattern == "" || !hasAllowedBotTag(entry.Tags) {
			continue
		}
		if _, err := regexp.Compile("(?i)" + entry.Pattern); err != nil {
			continue
		}
		valid = append(valid, entry.Pattern)
	}
	if len(valid) == 0 {
		return nil
	}

	combined, err := regexp.Compile("(?i)(?:" + strings.Join(valid, ")|(?:") + ")")
	if err != nil {
		return nil
	}
	return combined
}

func hasAllowedBotTag(tags []string) bool {
	for _, t := range tags {
		if _, ok := defaultBotTags[t]; ok {
			return true
		}
	}
	return false
}

const defaultBotPageFile = "bot.html"

// defaultBotUserAgents is the built-in list of lowercase User-Agent substrings
// identifying common search engine crawlers, social/link unfurlers, and AI bots.
var defaultBotUserAgents = []string{
	// Search engines
	"googlebot",
	"google-inspectiontool",
	"bingbot",
	"slurp",
	"duckduckbot",
	"baiduspider",
	"yandexbot",
	"sogou",
	"exabot",
	"applebot",
	"petalbot",
	"semrushbot",
	"ahrefsbot",
	// Social and link preview unfurlers
	"facebookexternalhit",
	"facebot",
	"twitterbot",
	"linkedinbot",
	"slackbot",
	"discordbot",
	"whatsapp",
	"telegrambot",
	"pinterestbot",
	"redditbot",
	"embedly",
	"quora link preview",
	"vkshare",
	"w3c_validator",
	"skypeuripreview",
	"bluesky",
	"mastodon",
	// AI crawlers
	"gptbot",
	"oai-searchbot",
	"chatgpt-user",
	"claudebot",
	"anthropic-ai",
	"perplexitybot",
	"bytespider",
	"ccbot",
	// Audits
	"lighthouse",
}

// CompiledBotPage holds precomputed bot detection patterns and sanitized file path
// for a Website route to avoid per-request allocations on the serving hot path.
type CompiledBotPage struct {
	file         string
	patterns     []string
	defaultRegex *regexp.Regexp
}

// CompileBotPage resolves a BotPageSpec into a precomputed matcher.
// Returns nil when bot page serving is disabled or no patterns are configured.
func CompileBotPage(spec *webappv1alpha1.BotPageSpec) *CompiledBotPage {
	if spec == nil {
		return nil
	}
	if spec.Enabled != nil && !*spec.Enabled {
		return nil
	}

	seen := make(map[string]struct{})
	var patterns []string
	addPattern := func(raw string) {
		p := strings.ToLower(strings.TrimSpace(raw))
		if p == "" {
			return
		}
		if _, dup := seen[p]; dup {
			return
		}
		seen[p] = struct{}{}
		patterns = append(patterns, p)
	}

	var defaultRegex *regexp.Regexp
	if !spec.OverrideDefaults {
		defaultRegex = defaultBotRegex
		for _, p := range defaultBotUserAgents {
			addPattern(p)
		}
	}
	for _, p := range spec.UserAgents {
		addPattern(p)
	}

	if len(patterns) == 0 && defaultRegex == nil {
		return nil
	}

	return &CompiledBotPage{
		file:         cleanBotPageFile(spec.File),
		patterns:     patterns,
		defaultRegex: defaultRegex,
	}
}

// File returns the sanitized relative path to the bot HTML file within the revision directory.
func (cb *CompiledBotPage) File() string {
	if cb == nil {
		return ""
	}
	return cb.file
}

// IsBot reports whether the given User-Agent header matches any configured bot pattern.
func (cb *CompiledBotPage) IsBot(userAgent string) bool {
	if cb == nil || userAgent == "" {
		return false
	}
	if cb.defaultRegex != nil && cb.defaultRegex.MatchString(userAgent) {
		return true
	}
	lowerUA := strings.ToLower(userAgent)
	for _, pattern := range cb.patterns {
		if strings.Contains(lowerUA, pattern) {
			return true
		}
	}
	return false
}

// cleanBotPageFile normalizes the configured file path and strips traversal segments
// so the resolved path always stays within the unpacked revision directory.
func cleanBotPageFile(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultBotPageFile
	}
	cleaned := strings.TrimPrefix(filepath.Clean("/"+trimmed), "/")
	if cleaned == "" || cleaned == "." {
		return defaultBotPageFile
	}
	return cleaned
}
