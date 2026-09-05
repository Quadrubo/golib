package filter

// ParseExpression reads a filter through the EBNF AIP-160 links at
// https://google.aip.dev/assets/misc/ebnf-filtering.txt and returns nil for the
// empty filter the grammar admits. It accepts every production, so a caller
// serving a subset rejects the rest against the tree rather than the text.
func ParseExpression(filter string) (*Expression, error) {
	tokens, err := lex(filter)
	if err != nil {
		return nil, err
	}

	p := &parser{tokens: tokens}
	p.skipWhitespace()

	if p.peek().kind == tokenEOF {
		return nil, nil
	}

	expression, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	p.skipWhitespace()

	if tok := p.peek(); tok.kind != tokenEOF {
		return nil, errorAt(tok.pos, "%s joins no expression", tok.kind)
	}

	return expression, nil
}

type parser struct {
	tokens []token
	at     int
}

func (p *parser) peek() token { return p.tokens[p.at] }

// peekPast returns the first token past any whitespace, which the EBNF makes
// insignificant inside a restriction and significant between factors.
func (p *parser) peekPast() token {
	at := p.at
	for p.tokens[at].kind == tokenWhitespace {
		at++
	}

	return p.tokens[at]
}

func (p *parser) next() token {
	tok := p.tokens[p.at]
	if tok.kind != tokenEOF {
		p.at++
	}

	return tok
}

func (p *parser) skipWhitespace() {
	for p.tokens[p.at].kind == tokenWhitespace {
		p.at++
	}
}

// parseExpression reads expression : sequence {WS AND WS sequence}.
func (p *parser) parseExpression() (*Expression, error) {
	expression := &Expression{Position: p.peek().pos}

	for {
		sequence, err := p.parseSequence()
		if err != nil {
			return nil, err
		}

		expression.Sequences = append(expression.Sequences, sequence)

		if p.peek().kind != tokenWhitespace || p.peekPast().kind != tokenAnd {
			return expression, nil
		}

		p.skipWhitespace()

		and := p.next()
		if p.peek().kind != tokenWhitespace {
			return nil, errorAt(and.pos, `"AND" is not followed by whitespace`)
		}

		p.skipWhitespace()
	}
}

// parseSequence reads sequence : factor {WS factor}.
func (p *parser) parseSequence() (*Sequence, error) {
	sequence := &Sequence{Position: p.peek().pos}

	for {
		factor, err := p.parseFactor()
		if err != nil {
			return nil, err
		}

		sequence.Factors = append(sequence.Factors, factor)

		if p.peek().kind != tokenWhitespace {
			return sequence, nil
		}

		switch p.peekPast().kind {
		case tokenAnd, tokenRightParen, tokenComma, tokenEOF:
			return sequence, nil
		}

		p.skipWhitespace()
	}
}

// parseFactor reads factor : term {WS OR WS term}.
func (p *parser) parseFactor() (*Factor, error) {
	factor := &Factor{Position: p.peek().pos}

	for {
		term, err := p.parseTerm()
		if err != nil {
			return nil, err
		}

		factor.Terms = append(factor.Terms, term)

		if p.peek().kind != tokenWhitespace || p.peekPast().kind != tokenOr {
			return factor, nil
		}

		p.skipWhitespace()

		or := p.next()
		if p.peek().kind != tokenWhitespace {
			return nil, errorAt(or.pos, `"OR" is not followed by whitespace`)
		}

		p.skipWhitespace()
	}
}

// parseTerm reads term : [(NOT WS | MINUS)] simple. A NOT carrying no
// whitespace after it opens a function name, which parseComparable reads.
func (p *parser) parseTerm() (*Term, error) {
	term := &Term{Position: p.peek().pos}

	switch {
	case p.peek().kind == tokenNot && p.tokens[p.at+1].kind == tokenWhitespace:
		p.next()
		p.skipWhitespace()

		term.Negated = true
	case p.peek().kind == tokenMinus:
		p.next()

		term.Negated = true
	}

	simple, err := p.parseSimple()
	if err != nil {
		return nil, err
	}

	term.Simple = simple

	return term, nil
}

// parseSimple reads simple : restriction | composite.
func (p *parser) parseSimple() (*Simple, error) {
	simple := &Simple{Position: p.peek().pos}

	if p.peek().kind == tokenLeftParen {
		composite, err := p.parseComposite()
		if err != nil {
			return nil, err
		}

		simple.Composite = composite

		return simple, nil
	}

	restriction, err := p.parseRestriction()
	if err != nil {
		return nil, err
	}

	simple.Restriction = restriction

	return simple, nil
}

// parseComposite reads composite : LPAREN expression RPAREN.
func (p *parser) parseComposite() (*Expression, error) {
	open := p.next()
	p.skipWhitespace()

	expression, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	p.skipWhitespace()

	if p.next().kind != tokenRightParen {
		return nil, errorAt(open.pos, `the "(" is not closed`)
	}

	return expression, nil
}

// parseRestriction reads restriction : comparable [comparator arg], which the
// EBNF makes insensitive to the whitespace around the comparator.
func (p *parser) parseRestriction() (*Restriction, error) {
	restriction := &Restriction{Position: p.peek().pos}

	comparable, err := p.parseComparable()
	if err != nil {
		return nil, err
	}

	restriction.Comparable = comparable

	if p.peekPast().kind != tokenComparator {
		return restriction, nil
	}

	p.skipWhitespace()

	restriction.Comparator = p.next().value

	p.skipWhitespace()

	arg, err := p.parseArg()
	if err != nil {
		return nil, err
	}

	restriction.Arg = arg

	return restriction, nil
}

// parseComparable reads comparable : member | function, which a "(" following
// the dotted name separates.
func (p *parser) parseComparable() (*Comparable, error) {
	comparable := &Comparable{Position: p.peek().pos}

	first := p.peek()

	names, err := p.parseDotted()
	if err != nil {
		return nil, err
	}

	if p.peek().kind != tokenLeftParen {
		// A keyword opens name, which function has and member does not, so a
		// keyword reaching no "(" leaves the grammar with nothing to read it as.
		switch first.kind {
		case tokenAnd, tokenOr, tokenNot:
			return nil, errorAt(first.pos, "%s is not a value", first.kind)
		}

		comparable.Member = &Member{Position: names[0].Position, Value: names[0], Fields: names[1:]}

		return comparable, nil
	}

	function, err := p.parseFunction(names)
	if err != nil {
		return nil, err
	}

	comparable.Function = function

	return comparable, nil
}

// parseDotted reads the dot-joined values a member and a function name share,
// value {DOT field} and name {DOT name}.
func (p *parser) parseDotted() ([]Value, error) {
	value, err := p.parseField()
	if err != nil {
		return nil, err
	}

	values := []Value{value}

	for p.peek().kind == tokenDot {
		p.next()

		field, err := p.parseField()
		if err != nil {
			return nil, err
		}

		values = append(values, field)
	}

	return values, nil
}

// parseValue reads value : TEXT | STRING.
func (p *parser) parseValue() (Value, error) {
	tok := p.peek()

	switch tok.kind {
	case tokenText, tokenString:
		p.next()

		return Value{Position: tok.pos, Quoted: tok.kind == tokenString, Text: tok.value}, nil
	}

	return Value{}, errorAt(tok.pos, "%s is not a value", tok.kind)
}

// parseField reads field : value | keyword.
func (p *parser) parseField() (Value, error) {
	switch tok := p.peek(); tok.kind {
	case tokenAnd, tokenOr, tokenNot:
		p.next()

		return Value{Position: tok.pos, Text: tok.value}, nil
	}

	return p.parseValue()
}

// parseFunction reads function : name {DOT name} LPAREN [argList] RPAREN.
func (p *parser) parseFunction(name []Value) (*Function, error) {
	function := &Function{Position: name[0].Position, Name: name}

	open := p.next()
	p.skipWhitespace()

	if p.peek().kind == tokenRightParen {
		p.next()

		return function, nil
	}

	for {
		arg, err := p.parseArg()
		if err != nil {
			return nil, err
		}

		function.Args = append(function.Args, arg)

		p.skipWhitespace()

		if p.peek().kind != tokenComma {
			break
		}

		p.next()
		p.skipWhitespace()
	}

	if p.next().kind != tokenRightParen {
		return nil, errorAt(open.pos, `the "(" is not closed`)
	}

	return function, nil
}

// parseArg reads arg : comparable | composite, and signs a number the EBNF
// reaches through no argument production. See docs/filter.md.
func (p *parser) parseArg() (*Arg, error) {
	arg := &Arg{Position: p.peek().pos}

	if p.peek().kind == tokenLeftParen {
		composite, err := p.parseComposite()
		if err != nil {
			return nil, err
		}

		arg.Composite = composite

		return arg, nil
	}

	signed := p.peek().kind == tokenMinus
	if signed {
		p.next()
	}

	comparable, err := p.parseComparable()
	if err != nil {
		return nil, err
	}

	if signed {
		if comparable.Member == nil || comparable.Member.Value.Quoted {
			return nil, errorAt(arg.Position, `"-" opens no number`)
		}

		comparable.Member.Value.Text = "-" + comparable.Member.Value.Text
	}

	arg.Comparable = comparable

	return arg, nil
}
