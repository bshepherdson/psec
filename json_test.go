package psec

import "testing"

func buildJSONParser() *Grammar {
	g := NewGrammar()
	g.AddSymbol("START", SeqAt(1, Symbol("ws"), Symbol("jsonValue"), Symbol("ws")))
	g.AddSymbol("jsonValue", Alt(Symbol("array"), Symbol("object"),
		Symbol("null"), Symbol("bool"), Symbol("string"), Symbol("number")))

	g.AddSymbol("ws", ManyDrop(OneOf(" \t\r\n")))

	g.WithAction("null", Literal("null"), func(interface{}) interface{} {
		return nil
	})

	g.WithAction("bool", Alt(Literal("false"), Literal("true")), func(res interface{}) interface{} {
		s := res.(string)
		return s == "true"
	})

	g.AddSymbol("string",
		SeqAt(1, Literal("\""), Stringify(ManyTill(AnyChar(), Literal("\"")))))

	g.WithAction("number",
		Seq(Optional(OneOf("+-")), Stringify(Many1(Range('0', '9')))),
		func(res interface{}) interface{} {
			parts := res.([]interface{})
			negated := false
			if sign, ok := parts[0].(byte); ok {
				negated = sign == '-'
			}

			digits := parts[1].(string)
			total := 0
			for _, d := range digits {
				total = 10*total + int(d-'0')
			}
			if negated {
				total = -total
			}
			return total
		})

	g.AddSymbol("comma", Seq(Symbol("ws"), Literal(","), Symbol("ws")))

	g.WithAction("object",
		SeqAt(2, Literal("{"), Symbol("ws"),
			SepBy(Symbol("keyValue"), Symbol("comma")),
			Symbol("ws"), Literal("}")),
		func(res interface{}) interface{} {
			out := make(map[string]interface{})
			pairs := res.([]interface{})
			for _, p0 := range pairs {
				p := p0.(keyValue)
				out[p.key] = p.value
			}
			return out
		})

	g.WithAction("keyValue",
		Seq(Symbol("string"), Symbol("ws"), Literal(":"),
			Symbol("ws"), Symbol("jsonValue")),
		func(res interface{}) interface{} {
			parts := res.([]interface{})
			return keyValue{parts[0].(string), parts[4]}
		})

	g.AddSymbol("array",
		SeqAt(2, Literal("["), Symbol("ws"),
			SepBy(Symbol("jsonValue"), Symbol("comma")),
			Symbol("ws"), Literal("]")))

	return g
}

type keyValue struct {
	key   string
	value interface{}
}

var grammar = buildJSONParser()

func TestNumberParser(t *testing.T) {
	res := grammar.ParseString("77")
	if n, ok := res.(int); !ok || n != 77 {
		t.FailNow()
	}
}

func TestStringParser(t *testing.T) {
	res := grammar.ParseString("\"some string here \"")
	if s, ok := res.(string); !ok || s != "some string here " {
		t.FailNow()
	}
}

func TestBoolean(t *testing.T) {
	res1 := grammar.ParseString("false")
	if s, ok := res1.(bool); !ok || s != false {
		t.FailNow()
	}
	res2 := grammar.ParseString("true")
	if s, ok := res2.(bool); !ok || s != true {
		t.FailNow()
	}
}

func TestArray(t *testing.T) {
	res0 := grammar.ParseString("   [   77, \"str here\", false   ]   ")
	res := res0.([]interface{})

	if len(res) != 3 {
		t.FailNow()
	}
	if res[0].(int) != 77 {
		t.FailNow()
	}
	if res[1].(string) != "str here" {
		t.FailNow()
	}
	if res[2].(bool) != false {
		t.FailNow()
	}
}

func TestObject(t *testing.T) {
	res0 := grammar.ParseString("  { \"key1\" :   -19  , \"kek\":\"str\"}  ")
	res := res0.(map[string]interface{})

	if v, ok := res["key1"]; ok {
		if v.(int) != -19 {
			t.FailNow()
		}
	} else {
		t.FailNow()
	}

	if v, ok := res["kek"]; ok {
		if v.(string) != "str" {
			t.FailNow()
		}
	} else {
		t.FailNow()
	}
}

func TestNestedArrays(t *testing.T) {
	res0 := grammar.ParseString("[ 7, [0, 2] ]")
	res := res0.([]interface{})
	if n, ok := res[0].(int); !ok || n != 7 {
		t.FailNow()
	}

	inner, ok := res[1].([]interface{})
	if !ok {
		t.FailNow()
	}
	if n, ok := inner[0].(int); !ok || n != 0 {
		t.FailNow()
	}
	if n, ok := inner[1].(int); !ok || n != 2 {
		t.FailNow()
	}
}

func TestNestedObjects(t *testing.T) {
	res0 := grammar.ParseString("{ \"arr\": [1,-8], \"obj\":{\"k\":\"v\"}, \"empty\"  : {} }")
	res := res0.(map[string]interface{})

	arr := res["arr"].([]interface{})
	if n, ok := arr[0].(int); !ok || n != 1 {
		t.FailNow()
	}
	if n, ok := arr[1].(int); !ok || n != -8 {
		t.FailNow()
	}

	obj := res["obj"].(map[string]interface{})
	if v, ok := obj["k"].(string); !ok || v != "v" {
		t.FailNow()
	}

	empty := res["empty"].(map[string]interface{})
	if len(empty) != 0 {
		t.FailNow()
	}
}
