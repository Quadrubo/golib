package filter

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind int

const (
	tokenEOF tokenKind = iota
	tokenWhitespace
	tokenText
	tokenString
	tokenAnd
	tokenOr
	tokenNot
	tokenComparator
	tokenLeftParen
	tokenRightParen
	tokenComma
	tokenDot
	tokenMinus
)

func (k tokenKind) String() string {
	switch k {
	case tokenEOF:
		return "the end of the filter"
	case tokenWhitespace:
		return "whitespace"
	case tokenText:
		return "text"
	case tokenString:
		return "a string"
	case tokenAnd:
		return `"AND"`
	case tokenOr:
		return `"OR"`
	case tokenNot:
		return `"NOT"`
	case tokenComparator:
		return "a comparator"
	case tokenLeftParen:
		return `"("`
	case tokenRightParen:
		return `")"`
	case tokenComma:
		return `","`
	case tokenDot:
		return `"."`
	case tokenMinus:
		return `"-"`
	}

	return "a token"
}

type token struct {
	kind tokenKind
	// value holds the source text, unquoted and unescaped for a string.
	value string
	// pos is a 1-based byte offset into the filter.
	pos int
}

// operators holds the runes that end a text token, since the EBNF definition of
// TEXT cannot be separated from the operators around it.
const operators = `()-.=:<>!,'"`

func lex(filter string) ([]token, error) {
	// A filter reaches the tokens whole or not at all. An invalid byte inside
	// quotes would otherwise reach the value as the replacement character the
	// client did not write, and outside them would bind raw.
	for at := 0; at < len(filter); {
		r, width := utf8.DecodeRuneInString(filter[at:])
		if r == utf8.RuneError && width == 1 {
			return nil, errorAt(at+1, "the filter is not valid UTF-8")
		}

		at += width
	}

	tokens := make([]token, 0, len(filter)/2+2)

	for at := 0; at < len(filter); {
		tok, next, err := lexOne(filter, at)
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, tok)
		at = next
	}

	return append(tokens, token{kind: tokenEOF, pos: len(filter) + 1}), nil
}

func lexOne(filter string, at int) (token, int, error) {
	pos := at + 1

	r, width := utf8.DecodeRuneInString(filter[at:])

	switch r {
	case '(':
		return token{kind: tokenLeftParen, value: "(", pos: pos}, at + width, nil
	case ')':
		return token{kind: tokenRightParen, value: ")", pos: pos}, at + width, nil
	case ',':
		return token{kind: tokenComma, value: ",", pos: pos}, at + width, nil
	case '.':
		return token{kind: tokenDot, value: ".", pos: pos}, at + width, nil
	case '-':
		return token{kind: tokenMinus, value: "-", pos: pos}, at + width, nil
	case '=', ':':
		return token{kind: tokenComparator, value: string(r), pos: pos}, at + width, nil
	case '<', '>':
		if strings.HasPrefix(filter[at+width:], "=") {
			return token{kind: tokenComparator, value: string(r) + "=", pos: pos}, at + width + 1, nil
		}

		return token{kind: tokenComparator, value: string(r), pos: pos}, at + width, nil
	case '!':
		if !strings.HasPrefix(filter[at+width:], "=") {
			return token{}, 0, errorAt(pos, `"!" opens no comparator other than "!="`)
		}

		return token{kind: tokenComparator, value: "!=", pos: pos}, at + width + 1, nil
	case '\'', '"':
		return lexString(filter, at, r)
	}

	if unicode.IsSpace(r) {
		end := at + width
		for end < len(filter) {
			next, w := utf8.DecodeRuneInString(filter[end:])
			if !unicode.IsSpace(next) {
				break
			}

			end += w
		}

		return token{kind: tokenWhitespace, value: filter[at:end], pos: pos}, end, nil
	}

	end := at
	for end < len(filter) {
		next, w := utf8.DecodeRuneInString(filter[end:])
		if unicode.IsSpace(next) || strings.ContainsRune(operators, next) {
			break
		}

		end += w
	}

	text := filter[at:end]

	switch text {
	case "AND":
		return token{kind: tokenAnd, value: text, pos: pos}, end, nil
	case "OR":
		return token{kind: tokenOr, value: text, pos: pos}, end, nil
	case "NOT":
		return token{kind: tokenNot, value: text, pos: pos}, end, nil
	}

	return token{kind: tokenText, value: text, pos: pos}, end, nil
}

func lexString(filter string, at int, quote rune) (token, int, error) {
	pos := at + 1

	var value strings.Builder

	for end := at + 1; end < len(filter); {
		r, width := utf8.DecodeRuneInString(filter[end:])

		switch r {
		case quote:
			return token{kind: tokenString, value: value.String(), pos: pos}, end + width, nil
		case '\\':
			if end+width >= len(filter) {
				return token{}, 0, errorAt(pos, "the string is not closed")
			}

			escaped, next, err := unescape(filter, end+width)
			if err != nil {
				return token{}, 0, err
			}

			value.WriteRune(escaped)

			end = next
		default:
			value.WriteRune(r)

			end += width
		}
	}

	return token{}, 0, errorAt(pos, "the string is not closed")
}

// unescape returns the rune a backslash escape stands for, over the set this
// service fixes because the EBNF leaves string escapes undefined.
func unescape(filter string, at int) (rune, int, error) {
	r, width := utf8.DecodeRuneInString(filter[at:])

	switch r {
	case '\\', '\'', '"':
		return r, at + width, nil
	case 'n':
		return '\n', at + width, nil
	case 'r':
		return '\r', at + width, nil
	case 't':
		return '\t', at + width, nil
	}

	return 0, 0, errorAt(at, `\%c is not an escape sequence`, r)
}
