package psec

import (
	"fmt"
	"strings"
)

// Parser is the common interface for all parsers, which consume streams and
// decorate them with values.
type Parser interface {
	// Parse consumes a Stream and symbolTable and returns a new Stream on success,
	// and nil on failure.
	Parse(Stream, symbolTable) (Stream, *parseError)
}

type parseError struct {
	expected []string
	message  string
	loc      *Loc
}

func (l *Loc) mkErrorExpectations(expected []string) *parseError {
	return &parseError{
		expected: expected,
		loc:      l,
	}
}

func (l *Loc) mkErrorExpect(expect string, args ...interface{}) *parseError {
	return l.mkErrorExpectations([]string{fmt.Sprintf(expect, args...)})
}

func (l *Loc) mkErrorMessage(msg string, args ...interface{}) *parseError {
	return &parseError{
		message: fmt.Sprintf(msg, args...),
		loc:     l,
	}
}

func (e *parseError) Error() string {
	prefix := fmt.Sprintf("%s line %d col %d",
		e.loc.Filename, e.loc.Line, e.loc.Col)

	if len(e.expected) == 1 {
		if e.message == "" {
			return fmt.Sprintf("%s: expected %s", prefix, e.expected[0])
		} else {
			return fmt.Sprintf("%s: %s, expected %s", prefix, e.message, e.expected[0])
		}
	} else if len(e.expected) > 1 {
		if e.message == "" {
			return fmt.Sprintf("%s: expected one of %s",
				prefix, strings.Join(e.expected, ", "))
		} else {
			return fmt.Sprintf("%s: %s, expected one of %s",
				prefix, e.message, strings.Join(e.expected, ", "))
		}
	} else {
		return fmt.Sprintf("%s: %s", prefix, e.message)
	}
}

// Action is a function type that adapts parser results from raw results to more
// meaningful results, such as AST nodes.
type Action func(results interface{}) (interface{}, error)

// symbolTable maps rule names to their parsers.
type symbolTable map[string]Parser

// Stream is an abstract stream of bytes, with an optional value.
// These are treated as immutable, so Tail() and SetValue() return new Streams.
type Stream interface {
	Head() (byte, bool) // Returns (b, false) or (0, true) at EOF.
	Tail() Stream
	Value() interface{}
	SetValue(interface{}) Stream
	Loc() *Loc
}

type Loc struct {
	Filename string
	Line     int
	Col      int
}

// The string is immutable and we have an index into it.
// The value might be nil, but it might also be some other value.
// stringPS is treated as immutable; that's why Tail() and SetValue()
// both return a new Stream.
type stringPS struct {
	str      string
	pos      uint
	filename string
	line     int
	col      int
	value    interface{}
	tail     *stringPS
}

func (s *stringPS) Head() (byte, bool) {
	if s.pos >= uint(len(s.str)) {
		return 0, true
	}
	return s.str[s.pos], false
}

func (s *stringPS) Tail() Stream {
	if s.tail == nil {
		s.tail = &stringPS{
			str:      s.str,
			pos:      s.pos + 1,
			filename: s.filename,
			line:     s.line,
			col:      s.col,
		}

		// If the character we just skipped was a newline, bump the line.
		if s.str[s.pos] == '\n' {
			s.tail.line = s.line + 1
			s.tail.col = 0
		}
	}
	return s.tail
}
func (s *stringPS) Value() interface{} { return s.value }
func (s *stringPS) SetValue(v interface{}) Stream {
	dup := *s
	dup.value = v
	return &dup
}

func (s *stringPS) Loc() *Loc {
	return &Loc{Filename: s.filename, Line: s.line, Col: s.col}
}

// The built-in Parsers themselves.

// Literal parses a given string exactly, matching case.
// The parser's value is the string itself.
// See LiteralIC for case-insensitive.
func Literal(str string) Parser {
	return &pLiteral{str}
}

type pLiteral struct {
	target string
}

func (p *pLiteral) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	i := 0
	for i < len(p.target) {
		h, eof := ps.Head()
		if eof || p.target[i] != h {
			return nil, ps.Loc().mkErrorExpect("literal '%s'", p.target)
		}
		ps = ps.Tail()
		i++
	}
	return ps.SetValue(p.target), nil
}

// TODO: Literal with value? I don't know how often that's actually used.

// LiteralIC parses a given string, ignoring case.
// The parser's value is the *original, canonical string*. (That is, the string
// passed to LiteralIC, not the capitalization in the input string.)
func LiteralIC(str string) Parser {
	return &pLiteralIC{str, strings.ToUpper(str)}
}

type pLiteralIC struct {
	target  string
	upcased string
}

func (p *pLiteralIC) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	for i := 0; i < len(p.target); i++ {
		h, eof := ps.Head()
		if eof || p.upcased[i] != strings.ToUpper(string(h))[0] {
			return nil, ps.Loc().mkErrorExpect("literal '%s'", p.target)
		}
		ps = ps.Tail()
	}
	return ps.SetValue(p.target), nil
}

// Alt accepts any number of parsers. It tries each one in turn. The first
// one to succeed becomes the resulting parse. If none of the parsers succeeds
// (or none are provided), Alt fails.
func Alt(parsers ...Parser) Parser {
	return &pAlt{parsers}
}

type pAlt struct {
	parsers []Parser
}

func (p *pAlt) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	var errs []*parseError
	for _, inner := range p.parsers {
		ret, err := inner.Parse(ps, g)
		if ret != nil {
			return ret, nil
		}
		errs = append(errs, err)
	}

	// We combine all the expectations of the inner errors together.
	var exps []string
	for _, err := range errs {
		exps = append(exps, err.expected...)
	}
	return nil, ps.Loc().mkErrorExpectations(exps)
}

// Seq runs an list of parsers in order, one after the other.
// If each parser succeeds, returns an array of their values.
// If any child parser fails, so does Seq.
func Seq(parsers ...Parser) Parser {
	return &pSeq{parsers}
}

type pSeq struct {
	parsers []Parser
}

func (p *pSeq) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	out := make([]interface{}, len(p.parsers))
	var err *parseError
	for i, inner := range p.parsers {
		ps, err = inner.Parse(ps, g)
		if err != nil {
			return nil, err
		}
		out[i] = ps.Value()
	}
	return ps.SetValue(out), nil
}

// SeqAt runs a list of parsers in order, one after the other.
// It takes an index (0-based), and its value is the value of that parser.
// If any of the parsers fails, so does SeqAt.
func SeqAt(index int, parsers ...Parser) Parser {
	return &pSeqAt{parsers, index}
}

type pSeqAt struct {
	parsers []Parser
	index   int
}

func (p *pSeqAt) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	var v interface{}
	var err *parseError
	for i, inner := range p.parsers {
		ps, err = inner.Parse(ps, g)
		if err != nil {
			return nil, err
		}
		if i == p.index {
			v = ps.Value()
		}
	}
	return ps.SetValue(v), nil
}

// Stringify wraps another parser, and combines its output (which should be a
// []byte) into a single string.
func Stringify(p Parser) Parser {
	return parserWithAction(p, func(raw interface{}) (interface{}, error) {
		res := raw.([]interface{})
		out := make([]byte, len(res))
		for i, c := range res {
			out[i] = c.(byte)
		}
		return string(out), nil
	})
}

// Optional attempts to run its inner parser. If that parser succeeds, Optional
// succeeds with its value. If the inner parser fails, Optional succeeds with
// value nil, and without consuming any input.
func Optional(p Parser) Parser {
	return &pOptional{p}
}

type pOptional struct {
	inner Parser
}

func (p *pOptional) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	res, _ := p.inner.Parse(ps, g)
	if res != nil {
		return res, nil
	}
	return ps.SetValue(nil), nil
}

// AnyChar parses any single character, returning it as the value.
func AnyChar() Parser {
	return &anyCharSingleton
}

type pAnyChar struct{}

var anyCharSingleton pAnyChar

func (p *pAnyChar) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	c, eof := ps.Head()
	if eof {
		return nil, ps.Loc().mkErrorMessage("unexpected EOF")
	}
	return ps.Tail().SetValue(c), nil
}

// OneOf matches any single character from a string of possibilities.
// Its value is that single character as a byte.
func OneOf(options string) Parser {
	return &pOneOf{options}
}

type pOneOf struct {
	options string
}

func (p *pOneOf) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	c, eof := ps.Head()
	if eof {
		return nil, ps.Loc().mkErrorMessage("unexpected EOF, expected one of '%s'", p.options)
	}
	for i := 0; i < len(p.options); i++ {
		if c == p.options[i] {
			return ps.Tail().SetValue(c), nil
		}
	}
	return nil, ps.Loc().mkErrorMessage("expected one of: %s", p.options)
}

// NoneOf matches any single character NOT in a "blacklist" string.
// Its value is the single character as a byte.
func NoneOf(blacklist string) Parser {
	return &pNoneOf{blacklist}
}

type pNoneOf struct {
	blacklist string
}

func (p *pNoneOf) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	c, eof := ps.Head()
	if eof {
		return nil, ps.Loc().mkErrorMessage("unexpected EOF")
	}
	for i := 0; i < len(p.blacklist); i++ {
		if c == p.blacklist[i] {
			return nil, ps.Loc().mkErrorMessage("unexpected %c", c)
		}
	}
	return ps.Tail().SetValue(c), nil
}

// Range takes two characters (bytes) and parses any character in that range
// (inclusive).
// For example, given 'a' and 'z', parses any lowercase letter.
// Value is the parsed character. Fails on EOF.
func Range(lo, hi byte) Parser {
	return &pRange{lo, hi}
}

type pRange struct {
	lo, hi byte
}

func (p *pRange) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	c, eof := ps.Head()
	if !eof && p.lo <= c && c <= p.hi {
		return ps.Tail().SetValue(c), nil
	}
	return nil, ps.Loc().mkErrorExpect("range(%c..%c)", p.lo, p.hi)
}

// Many parses 0 or more copies of its inner parser, returning an array of its
// results.
func Many(p Parser) Parser {
	return ManyMin(p, 0)
}

// Many1 is a variant of Many that parses 1 or more copies.
func Many1(p Parser) Parser {
	return ManyMin(p, 1)
}

// ManyMin is a variant of Many with a user-defined minimum.
func ManyMin(p Parser, min int) Parser {
	return &pMany{p, min, true}
}

// ManyDrop parses 0 or more copies, but discards the results rather than
// building an array.
func ManyDrop(p Parser) Parser {
	return &pMany{p, 0, false}
}

type pMany struct {
	inner   Parser
	min     int
	capture bool
}

// Combined parser for the different flavours of Many.
func (p *pMany) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	var results []interface{}
	if p.capture {
		results = make([]interface{}, 0)
	}

	found := 0
	var ps2 Stream
	var err *parseError
	for {
		ps2, err = p.inner.Parse(ps, g)
		if err != nil {
			break
		}
		found++
		if p.capture {
			results = append(results, ps2.Value())
		}
		ps = ps2
	}

	// Check that we've got at least min results.
	if found < p.min {
		return nil, &parseError{
			loc:      ps.Loc(),
			message:  fmt.Sprintf("minimum %d", p.min),
			expected: err.expected,
		}
	}

	// Good to return.
	if p.capture {
		return ps.SetValue(results), nil
	}
	return ps.SetValue(nil), nil
}

// SepBy matches 0 or more of one parser, separated by a second parser.
// The value is a list of the first parser's results.
// Does NOT consume a trailing separator.
func SepBy(p, sep Parser) Parser {
	return &pSepBy{p, sep, 0}
}

// SepBy1 matches 1 or more of one parser, separated by a second parser.
// The value is a list of the first parser's results.
// Does NOT consume a trailing separator.
func SepBy1(p, sep Parser) Parser {
	return &pSepBy{p, sep, 1}
}

type pSepBy struct {
	inner, sep Parser
	min        int
}

func (p *pSepBy) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	results := make([]interface{}, 0)

	var last Stream
	var err error
	for ps != nil {
		last = ps
		ps, err = p.inner.Parse(ps, g)
		if ps != nil {
			results = append(results, ps.Value())
		} else {
			break
		}
		last = ps
		ps, err = p.sep.Parse(ps, g)
	}

	if p.min > len(results) {
		return nil, ps.Loc().mkErrorMessage(
			"expected at least %d: %v", p.min, err)
	}

	return last.SetValue(results), nil
}

// EndBy matches 0 or more of one parser, each followed by a second parser.
// The value is a list of the first parser's results.
func EndBy(p, sep Parser) Parser {
	return &pEndBy{p, sep, 0}
}

// EndBy1 matches 1 or more of one parser, each followed by a second parser.
// The value is a list of the first parser's results.
func EndBy1(p, sep Parser) Parser {
	return &pEndBy{p, sep, 1}
}

type pEndBy struct {
	inner, sep Parser
	min        int
}

func (p *pEndBy) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	results := make([]interface{}, 0)

	var last Stream
	var err *parseError
	for ps != nil {
		last = ps
		ps, err = p.inner.Parse(ps, g)
		if ps == nil {
			break
		}
		results = append(results, ps.Value())
		ps, err = p.sep.Parse(ps, g)
	}

	if p.min > len(results) {
		return nil, ps.Loc().mkErrorMessage(
			"expected at least %d: %v", p.min, err)
	}

	return last.SetValue(results), nil
}

// ManyTill finds 0 or more instances of one parser, until it finds a
// terminator.
// This is "non-greedy", in the regular expression sense. It tries to parse the
// terminator repeatedly, and as soon as it succeeds ManyTill returns.
//
// Only when parsing the terminator fails does it try to parse the real parser.
//
// The output value is a slice of the main parser's results, possibly empty.
// If both the terminator and main parser fail at the same point, ManyTill
// fails.
//
// Some examples uses:
// string: SeqAt(1, Literal("\""), ManyTill(AnyChar(), Literal("\"")))
// lineComment: Seq(Literal("//"), ManyTill(AnyChar(), Literal("\n")))
func ManyTill(inner, terminator Parser) Parser {
	return &pManyTill{inner, terminator}
}

type pManyTill struct {
	inner, terminator Parser
}

func (p *pManyTill) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	results := make([]interface{}, 0)
	for {
		tps, err := p.terminator.Parse(ps, g)
		if tps != nil {
			return tps.SetValue(results), nil
		}
		ps, err = p.inner.Parse(ps, g)
		if err != nil {
			return nil, ps.Loc().mkErrorMessage(
				"failed to parse many %v", err)
		}
		results = append(results, ps.Value())
	}
}

func parserWithAction(p Parser, act Action) Parser {
	return &pWithAction{p, act}
}

type pWithAction struct {
	inner  Parser
	action Action
}

func (p *pWithAction) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	ps, err := p.inner.Parse(ps, g)
	if err != nil {
		return nil, err
	}
	res, e := p.action(ps.Value())
	if e != nil {
		return nil, ps.Loc().mkErrorMessage(e.Error())
	}
	return ps.SetValue(res), nil
}

// Symbol runs another parser in the grammar by name.
func Symbol(name string) Parser {
	return &pSymbol{name}
}

type pSymbol struct {
	name string
}

func (p *pSymbol) Parse(ps Stream, g symbolTable) (Stream, *parseError) {
	if inner, ok := g[p.name]; ok {
		return inner.Parse(ps, g)
	}
	// This is a programming error, not a problem with the user input, so a panic
	// is an appropriate reaction.
	panic(fmt.Sprintf("no symbol named '%s'", p.name))
}

// Grammar represents a complete parsing system: a set of symbols, a start
// symbol, a set of actions.
// Deliberately opaque.
type Grammar struct {
	symbols     symbolTable
	startSymbol string
}

// NewGrammar builds an empty grammar, with the conventional start symbol
// 'START'.
func NewGrammar() *Grammar {
	return &Grammar{make(map[string]Parser), "START"}
}

// AddSymbol adds or overwrites a symbol in the grammar.
func (g *Grammar) AddSymbol(name string, p Parser) {
	g.symbols[name] = p
}

// AddSymbols adds each symbol in a map to the grammar.
func (g *Grammar) AddSymbols(syms map[string]Parser) {
	for k, v := range syms {
		g.AddSymbol(k, v)
	}
}

// AddAction adds an action to a symbol's parser, wrapping any existing Action.
func (g *Grammar) AddAction(name string, action Action) {
	if p, ok := g.symbols[name]; ok {
		g.symbols[name] = parserWithAction(p, action)
	}
	panic(fmt.Sprintf("no such symbol: '%s'", name))
}

// WithAction adds a new symbol and an action for it at the same time, replacing
// any previous parser with that name.
func (g *Grammar) WithAction(name string, p Parser, action Action) {
	g.symbols[name] = parserWithAction(p, action)
}

// ParseString is the main entry point.
// It parses the input string. Returns the parse value on success, and nil on
// failure. (That means a Value of nil can't be distinguished from failure, but
// that's not a problem in practice.)
func (g *Grammar) ParseString(filename, str string) (interface{}, error) {
	return g.ParseStringWith(filename, str, "START")
}

func (g *Grammar) ParseStringWith(filename, str, startSym string) (interface{}, error) {
	var ps Stream = &stringPS{
		str:      str,
		pos:      0,
		filename: filename,
		line:     1,
		col:      0,
		value:    nil,
		tail:     nil,
	}

	if p, ok := g.symbols[startSym]; ok {
		ps, err := p.Parse(ps, g.symbols)
		if err != nil {
			return nil, err
		}

		_, eof := ps.Head()
		if !eof {
			return nil, ps.Loc().mkErrorMessage("incomplete parse, expected EOF but input remains")
		}

		return ps.Value(), nil
	}
	panic(fmt.Sprintf("start symbol '%s' does not exist", startSym))
}
