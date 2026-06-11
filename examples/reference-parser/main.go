// Command reference-parser is an ILLUSTRATIVE-ONLY Pratt (top-down operator
// precedence) parser and tree-walking evaluator for arithmetic and boolean
// expressions over float64 variables.
//
// IT EXISTS ONLY TO SHOW WHAT golem DELIBERATELY DOES NOT BUILD. golem stands on
// github.com/expr-lang/expr for exactly this layer — lexer, parser, precedence,
// operators, evaluation — so the production package contains none of the code
// below. This file is package main, is never imported by golem, and is not part
// of the public API. See the README in this directory.
//
// Run it with:
//
//	go run ./examples/reference-parser
package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// --- Lexer ---------------------------------------------------------------

type kind int

const (
	tEOF kind = iota
	tNum
	tIdent
	tOp
	tLParen
	tRParen
)

type token struct {
	kind kind
	text string
	num  float64
}

func lex(src string) ([]token, error) {
	var toks []token
	rs := []rune(src)
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '(':
			toks = append(toks, token{kind: tLParen})
			i++
		case c == ')':
			toks = append(toks, token{kind: tRParen})
			i++
		case unicode.IsDigit(c) || c == '.':
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.') {
				j++
			}
			n, err := strconv.ParseFloat(string(rs[i:j]), 64)
			if err != nil {
				return nil, fmt.Errorf("bad number %q", string(rs[i:j]))
			}
			toks = append(toks, token{kind: tNum, num: n})
			i = j
		case unicode.IsLetter(c):
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j])) {
				j++
			}
			toks = append(toks, token{kind: tIdent, text: string(rs[i:j])})
			i = j
		default:
			// Multi-rune operators first, then single-rune.
			two := ""
			if i+1 < len(rs) {
				two = string(rs[i : i+2])
			}
			if two == "&&" || two == "||" || two == "==" || two == "!=" || two == ">=" || two == "<=" {
				toks = append(toks, token{kind: tOp, text: two})
				i += 2
				continue
			}
			if strings.ContainsRune("+-*/%<>!", c) {
				toks = append(toks, token{kind: tOp, text: string(c)})
				i++
				continue
			}
			return nil, fmt.Errorf("unexpected character %q", string(c))
		}
	}
	return append(toks, token{kind: tEOF}), nil
}

// --- Pratt parser + tree-walking evaluator -------------------------------

// binding power per infix operator (higher binds tighter).
var bp = map[string]int{
	"||": 10, "&&": 20,
	"==": 30, "!=": 30, "<": 30, ">": 30, "<=": 30, ">=": 30,
	"+": 40, "-": 40,
	"*": 50, "/": 50, "%": 50,
}

type parser struct {
	toks []token
	pos  int
	vars map[string]float64
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

// parseExpr is the Pratt loop: parse a prefix operand, then fold infix operators
// whose binding power exceeds the caller's right-binding power.
func (p *parser) parseExpr(rbp int) (float64, error) {
	left, err := p.prefix()
	if err != nil {
		return 0, err
	}
	for {
		t := p.peek()
		if t.kind != tOp {
			break
		}
		lbp, ok := bp[t.text]
		if !ok || lbp <= rbp {
			break
		}
		p.next()
		right, err := p.parseExpr(lbp)
		if err != nil {
			return 0, err
		}
		left = apply(t.text, left, right)
	}
	return left, nil
}

func (p *parser) prefix() (float64, error) {
	t := p.next()
	switch t.kind {
	case tNum:
		return t.num, nil
	case tIdent:
		v, ok := p.vars[t.text]
		if !ok {
			return 0, fmt.Errorf("undefined variable %q", t.text)
		}
		return v, nil
	case tOp:
		if t.text == "-" || t.text == "!" {
			operand, err := p.parseExpr(60) // unary binds tighter than any infix
			if err != nil {
				return 0, err
			}
			if t.text == "-" {
				return -operand, nil
			}
			return b2f(operand == 0), nil
		}
	case tLParen:
		inner, err := p.parseExpr(0)
		if err != nil {
			return 0, err
		}
		if p.next().kind != tRParen {
			return 0, fmt.Errorf("expected )")
		}
		return inner, nil
	}
	return 0, fmt.Errorf("unexpected token %q", t.text)
}

// apply evaluates one binary operator. Booleans are encoded as 1.0/0.0 to keep
// the illustrative evaluator to a single float64 value type.
func apply(op string, a, b float64) float64 {
	switch op {
	case "+":
		return a + b
	case "-":
		return a - b
	case "*":
		return a * b
	case "/":
		return a / b
	case "%":
		return float64(int64(a) % int64(b))
	case "==":
		return b2f(a == b)
	case "!=":
		return b2f(a != b)
	case "<":
		return b2f(a < b)
	case ">":
		return b2f(a > b)
	case "<=":
		return b2f(a <= b)
	case ">=":
		return b2f(a >= b)
	case "&&":
		return b2f(a != 0 && b != 0)
	case "||":
		return b2f(a != 0 || b != 0)
	}
	return 0
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// eval lexes, parses, and evaluates src against vars in one call.
func eval(src string, vars map[string]float64) (float64, error) {
	toks, err := lex(src)
	if err != nil {
		return 0, err
	}
	p := &parser{toks: toks, vars: vars}
	v, err := p.parseExpr(0)
	if err != nil {
		return 0, err
	}
	if p.peek().kind != tEOF {
		return 0, fmt.Errorf("trailing tokens after expression")
	}
	return v, nil
}

func main() {
	vars := map[string]float64{"x": 5, "score": 0.91}
	for _, src := range []string{
		"2 + 3 * (x - 1)",
		"-x + 10",
		"score > 0.8 && x >= 5",
	} {
		v, err := eval(src, vars)
		if err != nil {
			fmt.Printf("%-22s => error: %v\n", src, err)
			continue
		}
		fmt.Printf("%-22s => %v\n", src, v)
	}
}
