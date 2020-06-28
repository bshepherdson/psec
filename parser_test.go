package psec

import (
	"fmt"
	"testing"
)

// Test for various specific parsing rules.
func TestAnyChar(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START", AnyChar())
	r, err := g.ParseString("test", "x")
	if err != nil {
		t.Errorf("unexpected failure: %v", err)
	}

	if r, ok := r.(byte); ok {
		if r != 'x' {
			t.Errorf("unexpected AnyChar(): %v", r)
		}
	} else {
		t.Errorf("AnyChar return was not a byte: %T", r)
	}
}

func TestLiteral(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START", Literal("a"))
	r, err := g.ParseString("test", "a")
	if err != nil {
		t.Errorf("unexpected failure: %v", err)
	}

	if r, ok := r.(string); ok {
		if r != "a" {
			t.Errorf("unexpected value from literal: %v", r)
		}
	} else {
		t.Errorf("Literal return was not a string: %T", r)
	}
}

func TestLiteralLong(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START", Literal("abcdef"))
	r, err := g.ParseString("test", "abcdef")
	if err != nil {
		t.Errorf("unexpected failure: %v", err)
	}

	if r, ok := r.(string); ok {
		if r != "abcdef" {
			t.Errorf("unexpected value from literal: %v", r)
		}
	} else {
		t.Errorf("Literal return was not a string: %T", r)
	}
}

func TestLiteralMismatch(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START", Literal("abcd"))
	_, err := g.ParseString("test", "abd")
	if err == nil {
		t.Errorf("Expected parse to fail")
	}
	if err.Error() != "test line 1 col 0: expected literal 'abcd'" {
		t.Errorf("wrong error: %v", err)
	}
}

func TestLiteralCase(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START", Literal("abc"))
	_, err := g.ParseString("test", "ABC")
	if err == nil {
		t.Errorf("expected failure")
	}

	if err.Error() != "test line 1 col 0: expected literal 'abc'" {
		t.Errorf("wrong error message: %v", err)
	}
}

func TestLiteralIC(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START", LiteralIC("abc"))
	r, err := g.ParseString("test", "ABC")
	if err != nil {
		t.Errorf("unexpected failure: %v", err)
	}

	if r, ok := r.(string); ok {
		if r != "abc" { // NB: It matches the hard-coded text, not the input.
			t.Errorf("mismatched return, got %v", r)
		}
	} else {
		t.Errorf("LiteralIC return was not a string: %T", r)
	}
}

func expectString(t *testing.T, g *Grammar, input, expected string) {
	r, err := g.ParseString("test", input)
	if err != nil {
		t.Errorf("unexpected failure: %v", err)
	}

	if r, ok := r.(string); ok {
		if r != expected {
			t.Errorf("mismatched return, got %v", r)
		}
	} else {
		t.Errorf("return was not a string: %#v %T", r, r)
	}
}

func expectNil(t *testing.T, g *Grammar, input string) {
	r, err := g.ParseString("test", input)
	if err != nil {
		t.Errorf("unexpected failure: %v", err)
	}

	if r != nil {
		t.Errorf("expected parsed value to be nil")
	}
}

func expectStrings(t *testing.T, g *Grammar, input string, expected []string) {
	rs, err := g.ParseString("test", input)
	if err != nil {
		t.Errorf("unexpected failure: %v", err)
		return
	}

	if rs, ok := rs.([]interface{}); ok {
		if len(rs) != len(expected) {
			t.Errorf("expected %d results, got %d", len(expected), len(rs))
		}

		for i := 0; i < len(expected); i++ {
			if r, ok := rs[i].(string); ok {
				if r != expected[i] {
					t.Errorf("expected result %d to be %v, got %v", i, expected[i], r)
				}
			} else {
				t.Errorf("expected string, but got %T", r)
			}
		}
	} else {
		t.Errorf("expected []interface{}, but got %T", rs)
	}
}

func expectError(t *testing.T, g *Grammar, input, expected string) {
	_, err := g.ParseString("test", input)
	if err == nil {
		t.Errorf("expected failure, but parsing succeeded")
	}

	s := fmt.Sprintf("test line 1 col 0: %s", expected)
	if err.Error() != s {
		t.Errorf("mismatched error message: %v", err)
		fmt.Printf("expected: %s\n", s)
	}
}

func TestAlt(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START", Alt(Literal("abc"), Literal("aaa"), Literal("def")))

	expectString(t, g, "abc", "abc")
	expectString(t, g, "aaa", "aaa")
	expectString(t, g, "def", "def")
	expectError(t, g, "ABC",
		"expected one of literal 'abc', literal 'aaa', literal 'def'")
}

func TestSeq(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START",
		Seq(Literal("["), Alt(Literal("a"), Literal("b")), Literal("]")))

	expectStrings(t, g, "[a]", []string{"[", "a", "]"})
	expectStrings(t, g, "[b]", []string{"[", "b", "]"})
	expectError(t, g, "[c]", "expected one of literal 'a', literal 'b'")
}

func TestSeqAt(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START",
		SeqAt(1, Literal("["), Alt(Literal("a"), Literal("b")), Literal("]")))
	expectString(t, g, "[a]", "a")
	expectString(t, g, "[b]", "b")
	expectError(t, g, "[c]", "expected one of literal 'a', literal 'b'")
	expectError(t, g, "[ab", "expected literal ']'")
}

func TestOptional(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START",
		SeqAt(2, Literal("["),
			Alt(Literal("a"), Literal("b")),
			Optional(Literal("?")),
			Literal("]")))

	expectNil(t, g, "[a]")
	expectString(t, g, "[b?]", "?")
}

func expectByte(t *testing.T, g *Grammar, input string, expected byte) {
	r, err := g.ParseString("test", input)
	if err != nil {
		t.Errorf("unexpected failure: %v", err)
	}

	if r, ok := r.(byte); ok {
		if r != expected {
			t.Errorf("mismatched return, got %v", r)
		}
	} else {
		t.Errorf("return was not a byte: %#v %T", r, r)
	}
}

func TestOneOf(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START", OneOf("abcd"))
	expectByte(t, g, "a", byte('a'))
	expectByte(t, g, "c", byte('c'))
	expectError(t, g, "f", "expected one of: abcd")
}

func TestNoneOf(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START", NoneOf("abcd"))
	expectByte(t, g, "f", byte('f'))
	expectByte(t, g, "z", byte('z'))
	expectError(t, g, "c", "unexpected c")
}

func TestRange(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START", Range('a', 'z'))
	expectByte(t, g, "f", byte('f'))
	expectByte(t, g, "a", byte('a'))
	expectByte(t, g, "z", byte('z'))
	expectError(t, g, "A", "expected range(a..z)")
}

func TestMany(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START", Stringify(Many(Range('a', 'z'))))
	expectString(t, g, "abc", "abc")
	expectString(t, g, "kds", "kds")
	expectString(t, g, "c", "c")
	expectString(t, g, "", "")
	expectError(t, g, "dsCC", "incomplete parse, expected EOF but input remains")
}

func TestManyMore(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START",
		SeqAt(1, Literal("["), Stringify(Many(Range('a', 'z'))), Literal("]")))
	expectString(t, g, "[abc]", "abc")
	expectString(t, g, "[]", "")
	expectError(t, g, "[A]", "expected literal ']'")
}

func TestMany1(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("START",
		SeqAt(1, Literal("["), Stringify(Many1(Range('a', 'z'))), Literal("]")))
	expectString(t, g, "[abc]", "abc")
	expectString(t, g, "[x]", "x")
	expectError(t, g, "[]", "minimum 1, expected range(a..z)")
	expectError(t, g, "[ccA]", "expected literal ']'")
}

func TestSepBy(t *testing.T) {
	g := NewGrammar()
	g.AddSymbol("chunk",
		SeqAt(1, Literal("["), Stringify(Many(Range('a', 'z'))), Literal("]")))
	g.AddSymbol("START", SepBy(Symbol("chunk"), Literal(",")))
	expectStrings(t, g, "[abc],[],[z]", []string{"abc", "", "z"})
	expectStrings(t, g, "[dd]", []string{"dd"})
	expectStrings(t, g, "", []string{})
	expectError(t, g, "[aaA],[dc]", "incomplete parse, expected EOF but input remains")
	expectError(t, g, "[aa]![dc]", "incomplete parse, expected EOF but input remains")
}
