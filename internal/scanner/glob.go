package scanner

import (
	"path/filepath"
	"regexp"
	"strings"
)

func globRegex(pattern string) (*regexp.Regexp, error) {
	pattern = filepath.ToSlash(filepath.Clean(filepath.FromSlash(pattern)))
	var builder strings.Builder
	builder.WriteString("^")

	for index := 0; index < len(pattern); {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				index += 2
				if index < len(pattern) && pattern[index] == '/' {
					builder.WriteString("(?:.*/)?")
					index++
				} else {
					builder.WriteString(".*")
				}
			} else {
				builder.WriteString("[^/]*")
				index++
			}
		case '?':
			builder.WriteString("[^/]")
			index++
		default:
			builder.WriteString(regexp.QuoteMeta(string(pattern[index])))
			index++
		}
	}
	builder.WriteString("$")
	return regexp.Compile(builder.String())
}

func matchesAny(path string, patterns []*regexp.Regexp) bool {
	path = filepath.ToSlash(path)
	for _, pattern := range patterns {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}
