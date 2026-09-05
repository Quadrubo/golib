package filter

// Expression is one or more sequences joined by AND, the loosest binding the
// grammar has.
type Expression struct {
	Position  int
	Sequences []*Sequence
}

// Sequence is one or more factors joined by whitespace alone.
type Sequence struct {
	Position int
	Factors  []*Factor
}

// Factor is one or more terms joined by OR, which binds tighter than both AND
// and a sequence.
type Factor struct {
	Position int
	Terms    []*Term
}

// Term is a simple expression that a leading NOT or - negates.
type Term struct {
	Position int
	Negated  bool
	Simple   *Simple
}

type Simple struct {
	Position    int
	Restriction *Restriction
	Composite   *Expression
}

// Restriction compares a comparable against an argument. Comparator and Arg
// stay empty where the restriction names a comparable alone, which AIP-160
// calls a global restriction.
type Restriction struct {
	Position   int
	Comparable *Comparable
	Comparator string
	Arg        *Arg
}

type Comparable struct {
	Position int
	Member   *Member
	Function *Function
}

// Member is a value followed by the fields a dot qualifies it with.
type Member struct {
	Position int
	Value    Value
	Fields   []Value
}

// Function applies a dot-qualified name to zero or more arguments.
type Function struct {
	Position int
	Name     []Value
	Args     []*Arg
}

type Arg struct {
	Position   int
	Comparable *Comparable
	Composite  *Expression
}

// Value is one TEXT or STRING token. Quoted separates the two, which decides
// whether a - or a . the value carries reached the lexer as an operator.
type Value struct {
	Position int
	Quoted   bool
	Text     string
}
