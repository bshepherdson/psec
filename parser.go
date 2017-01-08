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
	Parse(Stream, symbolTable) Stream
}

// Action is a function type that adapts parser results from raw results to more
// meaningful results, such as AST nodes.
type Action func(results interface{}) interface{}

// symbolTable maps rule names to their parsers.
type symbolTable map[string]Parser

// Stream is an abstract stream of bytes, with an optional value.
// These are treated as immutable, so Tail() and SetValue() return new Streams.
type Stream interface {
	Head() (byte, bool) // Returns (b, false) or (0, true) at EOF.
	Tail() Stream
	Value() interface{}
	SetValue(interface{}) Stream
}

// The string is immutable and we have an index into it.
// The value might be nil, but it might also be some other value.
// stringPS is treated as immutable; that's why Tail() and SetValue()
// both return a new Stream.
type stringPS struct {
	str   string
	pos   uint
	value interface{}
	tail  *stringPS
}

func (s *stringPS) Head() (byte, bool) {
	if s.pos >= uint(len(s.str)) {
		return 0, true
	}
	return s.str[s.pos], false
}

func (s *stringPS) Tail() Stream {
	if s.tail == nil {
		s.tail = &stringPS{s.str, s.pos + 1, nil, nil}
	}
	return s.tail
}
func (s *stringPS) Value() interface{} { return s.value }
func (s *stringPS) SetValue(v interface{}) Stream {
	dup := *s
	dup.value = v
	return &dup
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

func (p *pLiteral) Parse(ps Stream, g symbolTable) Stream {
	i := 0
	for i < len(p.target) {
		h, eof := ps.Head()
		if eof || p.target[i] != h {
			return nil
		}
		ps = ps.Tail()
		i++
	}
	return ps.SetValue(p.target)
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

func (p *pLiteralIC) Parse(ps Stream, g symbolTable) Stream {
	for i := 0; i < len(p.target); i++ {
		h, eof := ps.Head()
		if eof || p.upcased[i] != strings.ToUpper(string(h))[0] {
			return nil
		}
		ps = ps.Tail()
	}
	return ps.SetValue(p.target)
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

func (p *pAlt) Parse(ps Stream, g symbolTable) Stream {
	for _, inner := range p.parsers {
		ret := inner.Parse(ps, g)
		if ret != nil {
			return ret
		}
	}
	return nil
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

func (p *pSeq) Parse(ps Stream, g symbolTable) Stream {
	out := make([]interface{}, len(p.parsers))
	for i, inner := range p.parsers {
		ps = inner.Parse(ps, g)
		if ps == nil {
			return nil
		}
		out[i] = ps.Value()
	}
	return ps.SetValue(out)
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

func (p *pSeqAt) Parse(ps Stream, g symbolTable) Stream {
	var v interface{}
	for i, inner := range p.parsers {
		ps = inner.Parse(ps, g)
		if ps == nil {
			return nil
		}
		if i == p.index {
			v = ps.Value()
		}
	}
	return ps.SetValue(v)
}

// Stringify wraps another parser, and combines its output (which should be a
// []byte) into a single string.
func Stringify(p Parser) Parser {
	return parserWithAction(p, func(raw interface{}) interface{} {
		res := raw.([]interface{})
		out := make([]byte, len(res))
		for i, c := range res {
			out[i] = c.(byte)
		}
		return string(out)
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

func (p *pOptional) Parse(ps Stream, g symbolTable) Stream {
	res := p.inner.Parse(ps, g)
	if res != nil {
		return res
	}
	return ps.SetValue(nil)
}

// AnyChar parses any single character, returning it as the value.
func AnyChar() Parser {
	return &anyCharSingleton
}

type pAnyChar struct{}

var anyCharSingleton pAnyChar

func (p *pAnyChar) Parse(ps Stream, g symbolTable) Stream {
	c, eof := ps.Head()
	if eof {
		return nil
	}
	return ps.Tail().SetValue(c)
}

// OneOf matches any single character from a string of possibilities.
// Its value is that single character as a byte.
func OneOf(options string) Parser {
	return &pOneOf{options}
}

type pOneOf struct {
	options string
}

func (p *pOneOf) Parse(ps Stream, g symbolTable) Stream {
	c, eof := ps.Head()
	if eof {
		return nil
	}
	for i := 0; i < len(p.options); i++ {
		if c == p.options[i] {
			return ps.Tail().SetValue(c)
		}
	}
	return nil
}

// NoneOf matches any single character NOT in a "blacklist" string.
// Its value is the single character as a byte.
func NoneOf(blacklist string) Parser {
	return &pNoneOf{blacklist}
}

type pNoneOf struct {
	blacklist string
}

func (p *pNoneOf) Parse(ps Stream, g symbolTable) Stream {
	c, eof := ps.Head()
	if eof {
		return nil
	}
	for i := 0; i < len(p.blacklist); i++ {
		if c == p.blacklist[i] {
			return nil
		}
	}
	return ps.Tail().SetValue(c)
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

func (p *pRange) Parse(ps Stream, g symbolTable) Stream {
	c, eof := ps.Head()
	if !eof && p.lo <= c && c <= p.hi {
		return ps.Tail().SetValue(c)
	}
	return nil
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
func (p *pMany) Parse(ps Stream, g symbolTable) Stream {
	var results []interface{}
	if p.capture {
		results = make([]interface{}, 0)
	}

	found := 0
	for {
		ps2 := p.inner.Parse(ps, g)
		if ps2 == nil {
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
		return nil
	}

	// Good to return.
	if p.capture {
		return ps.SetValue(results)
	}
	return ps.SetValue(nil)
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

func (p *pSepBy) Parse(ps Stream, g symbolTable) Stream {
	results := make([]interface{}, 0)

	var last Stream
	for ps != nil {
		last = ps
		ps = p.inner.Parse(ps, g)
		if ps != nil {
			results = append(results, ps.Value())
		} else {
			break
		}
		last = ps
		ps = p.sep.Parse(ps, g)
	}

	if p.min > len(results) {
		return nil
	}

	return last.SetValue(results)
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

func (p *pEndBy) Parse(ps Stream, g symbolTable) Stream {
	results := make([]interface{}, 0)

	var last Stream
	for ps != nil {
		last = ps
		ps = p.inner.Parse(ps, g)
		if ps == nil {
			break
		}
		results = append(results, ps.Value())
		ps = p.sep.Parse(ps, g)
	}

	if p.min > len(results) {
		return nil
	}

	return last.SetValue(results)
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

func (p *pManyTill) Parse(ps Stream, g symbolTable) Stream {
	results := make([]interface{}, 0)
	for ps != nil {
		tps := p.terminator.Parse(ps, g)
		if tps != nil {
			return tps.SetValue(results)
		}
		ps = p.inner.Parse(ps, g)
		if ps != nil {
			results = append(results, ps.Value())
		}
	}

	// If we come down here, then both the terminator and body parsers failed.
	return nil
}

func parserWithAction(p Parser, act Action) Parser {
	return &pWithAction{p, act}
}

type pWithAction struct {
	inner  Parser
	action Action
}

func (p *pWithAction) Parse(ps Stream, g symbolTable) Stream {
	ps = p.inner.Parse(ps, g)
	if ps == nil {
		return nil
	}
	return ps.SetValue(p.action(ps.Value()))
}

// Symbol runs another parser in the grammar by name.
func Symbol(name string) Parser {
	return &pSymbol{name}
}

type pSymbol struct {
	name string
}

func (p *pSymbol) Parse(ps Stream, g symbolTable) Stream {
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
func (g *Grammar) ParseString(str string) interface{} {
	var ps Stream = &stringPS{str, 0, nil, nil}
	if p, ok := g.symbols[g.startSymbol]; ok {
		ps = p.Parse(ps, g.symbols)
		if ps == nil {
			return nil
		}
		return ps.Value()
	}
	panic(fmt.Sprintf("start symbol '%s' does not exist", g.startSymbol))
}
