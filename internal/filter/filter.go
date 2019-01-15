package filter

import (
	"regexp"
	"strings"
)

var matchGroups = regexp.MustCompile(`(?:([^\s"]+)|"([^"]*)"?)+`)

// Split terms by spaces, except for spaces within quotes.
// Like http://stackoverflow.com/questions/16261635/javascript-split-string-by-space-but-ignore-space-in-quotes-notice-not-to-spli
// except:
// - Treat the query ["san fran] as having the single term ["san fran"] instead of ["san", "fran"]
// - Removes the delimiting quotes.
func Match(str, query string) bool {
	if query == "" {
		return true
	}

	matches := matchGroups.FindAllStringSubmatch(query, -1)
	for _, match := range matches {
		// Empty quotes can count as a match, ignore them.
		if match[1] == "" && match[2] == "" {
			continue
		}
		// Get relevant match (one of these has to be a non-empty string, otherwise this wouldn't be
		// a match).
		matchString := match[1]
		if matchString == "" {
			matchString = match[2]
		}

		if strings.Contains(strings.ToLower(str), strings.ToLower(matchString)) {
			return true
		}
	}
	return false
}
