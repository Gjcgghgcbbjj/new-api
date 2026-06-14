package openaicompat

import (
	"fmt"
	"regexp"
	"sync"
)

var compiledRegexCache sync.Map // map[string]*regexp.Regexp

func ValidateRegexPatterns(patterns []string) error {
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("invalid regex pattern %q: %w", pattern, err)
		}
	}
	return nil
}

func matchAnyRegex(patterns []string, s string) bool {
	if len(patterns) == 0 || s == "" {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		re, ok := compiledRegexCache.Load(pattern)
		if !ok {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				// Treat invalid patterns as non-matching to avoid breaking runtime traffic.
				continue
			}
			re = compiled
			compiledRegexCache.Store(pattern, re)
		}
		if re.(*regexp.Regexp).MatchString(s) {
			return true
		}
	}
	return false
}
