package resourcename

import (
	"fmt"
	"regexp"
	"strings"
)

const wildcard = "*"

var segment = regexp.MustCompile(`^[a-z0-9-]{1,63}$`)

type Pattern struct {
	pattern     string
	collections []string
	singleton   string
}

// MustCompile reads a pattern alternating collection and wildcard, such as
// `rooms/*` or `lists/*/tasks/*`, and panics on anything else.
func MustCompile(pattern string) Pattern {
	segments := strings.Split(pattern, "/")
	if len(segments)%2 != 0 {
		panic("resourcename: pattern " + pattern + " must alternate collection and " + wildcard)
	}

	return Pattern{pattern: pattern, collections: collectionsOf(pattern, segments)}
}

// Singleton reads a pattern closing on the fixed name of a resource with one
// instance, such as `serverConfig` or `users/*/settings`, and panics on
// anything else.
func Singleton(pattern string) Pattern {
	segments := strings.Split(pattern, "/")
	last := len(segments) - 1

	if len(segments)%2 != 1 || segments[last] == "" || segments[last] == wildcard {
		panic("resourcename: pattern " + pattern + " must close on a singleton name")
	}

	return Pattern{
		pattern:     pattern,
		collections: collectionsOf(pattern, segments[:last]),
		singleton:   segments[last],
	}
}

func collectionsOf(pattern string, segments []string) []string {
	collections := make([]string, 0, len(segments)/2)
	for i := 0; i < len(segments); i += 2 {
		if segments[i] == "" || segments[i] == wildcard || segments[i+1] != wildcard {
			panic("resourcename: pattern " + pattern + " must alternate collection and " + wildcard)
		}
		collections = append(collections, segments[i])
	}

	return collections
}

func (p Pattern) Parse(name string) ([]string, error) {
	segments := strings.Split(name, "/")
	if len(segments) != p.size() {
		return nil, &Error{Name: name, Pattern: p.pattern}
	}

	ids := make([]string, 0, len(p.collections))
	for i, collection := range p.collections {
		if segments[i*2] != collection || !segment.MatchString(segments[i*2+1]) {
			return nil, &Error{Name: name, Pattern: p.pattern}
		}
		ids = append(ids, segments[i*2+1])
	}

	if p.singleton != "" && segments[len(segments)-1] != p.singleton {
		return nil, &Error{Name: name, Pattern: p.pattern}
	}

	return ids, nil
}

// Format panics on an id count the pattern does not take.
func (p Pattern) Format(ids ...string) string {
	if len(ids) != len(p.collections) {
		panic(fmt.Sprintf("resourcename: pattern %s cannot format %q", p.pattern, ids))
	}

	segments := make([]string, 0, p.size())
	for i, collection := range p.collections {
		segments = append(segments, collection, ids[i])
	}

	if p.singleton != "" {
		segments = append(segments, p.singleton)
	}

	return strings.Join(segments, "/")
}

func (p Pattern) size() int {
	size := len(p.collections) * 2
	if p.singleton != "" {
		size++
	}

	return size
}

type Error struct {
	Name    string
	Pattern string
}

func (e *Error) Error() string {
	if e.Name == "" {
		return "resource name is required, expected " + e.Pattern
	}

	return fmt.Sprintf("resource name %q must match %s", e.Name, e.Pattern)
}
