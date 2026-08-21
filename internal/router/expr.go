package router

import (
	"path"
	"strconv"
	"strings"
	"unicode"

	"device-telemetry-router/internal/domain"
)

// matchExpr evaluates an optional matcher expression over an event. The
// grammar is a whitespace-tolerant boolean expression over the event fields
// metric, device and value:
//
//	expr   := orExpr
//	orExpr := andExpr ( "or" andExpr )*
//	andExpr:= atom ( "and" atom )*
//	atom   := '(' expr ')' | cmp
//	cmp    := field op literal
//	op     := '==' | '!=' | '~'  ( '~' is glob matching for strings )
//
// An empty expression matches everything. Parse or type errors evaluate to
// false (no match) so a malformed rule never accidentally routes traffic.
func matchExpr(expr string, ev domain.Event) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true
	}
	p := &exprParser{s: expr, ev: ev}
	ok, err := p.parseOr()
	if err != nil {
		return false
	}
	p.skipSpace()
	if p.pos != len(p.s) {
		return false
	}
	return ok
}

type exprParser struct {
	s   string
	pos int
	ev  domain.Event
}

func (p *exprParser) skipSpace() {
	for p.pos < len(p.s) && unicode.IsSpace(rune(p.s[p.pos])) {
		p.pos++
	}
}

func (p *exprParser) peekKeyword(kw string) bool {
	p.skipSpace()
	if p.pos+len(kw) > len(p.s) {
		return false
	}
	if strings.EqualFold(p.s[p.pos:p.pos+len(kw)], kw) {
		end := p.pos + len(kw)
		return end == len(p.s) || !isWordChar(rune(p.s[end]))
	}
	return false
}

func (p *exprParser) consumeKeyword(kw string) bool {
	if p.peekKeyword(kw) {
		p.pos += len(kw)
		p.skipSpace()
		return true
	}
	return false
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func (p *exprParser) parseOr() (bool, error) {
	left, err := p.parseAnd()
	if err != nil {
		return false, err
	}
	for p.consumeKeyword("or") {
		right, err := p.parseAnd()
		if err != nil {
			return false, err
		}
		left = left || right
	}
	return left, nil
}

func (p *exprParser) parseAnd() (bool, error) {
	left, err := p.parseAtom()
	if err != nil {
		return false, err
	}
	for p.consumeKeyword("and") {
		right, err := p.parseAtom()
		if err != nil {
			return false, err
		}
		left = left && right
	}
	return left, nil
}

func (p *exprParser) parseAtom() (bool, error) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return false, &exprError{"unexpected end of expression"}
	}
	if p.s[p.pos] == '(' {
		p.pos++
		v, err := p.parseOr()
		if err != nil {
			return false, err
		}
		p.skipSpace()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return false, &exprError{"missing closing parenthesis"}
		}
		p.pos++
		return v, nil
	}
	return p.parseCmp()
}

func (p *exprParser) parseCmp() (bool, error) {
	field, err := p.parseIdent()
	if err != nil {
		return false, err
	}
	p.skipSpace()
	op, err := p.parseOp()
	if err != nil {
		return false, err
	}
	p.skipSpace()
	lit, err := p.parseLiteral()
	if err != nil {
		return false, err
	}
	return compareExpr(field, op, lit, p.ev)
}

func (p *exprParser) parseIdent() (string, error) {
	p.skipSpace()
	start := p.pos
	for p.pos < len(p.s) && isWordChar(rune(p.s[p.pos])) {
		p.pos++
	}
	if start == p.pos {
		return "", &exprError{"expected field name"}
	}
	return p.s[start:p.pos], nil
}

func (p *exprParser) parseOp() (string, error) {
	if p.pos+2 <= len(p.s) {
		two := p.s[p.pos : p.pos+2]
		switch two {
		case "==", "!=", ">=", "<=":
			p.pos += 2
			return two, nil
		}
	}
	if p.pos < len(p.s) {
		switch p.s[p.pos] {
		case '~':
			p.pos++
			return "~", nil
		case '=':
			p.pos++
			return "==", nil
		case '!':
			p.pos++
			return "!=", nil
		case '>':
			p.pos++
			return ">", nil
		case '<':
			p.pos++
			return "<", nil
		}
	}
	return "", &exprError{"expected comparison operator (==, !=, ~, >, <, >=, <=)"}
}

func (p *exprParser) parseLiteral() (string, error) {
	p.skipSpace()
	if p.pos < len(p.s) && (p.s[p.pos] == '"' || p.s[p.pos] == '\'') {
		quote := p.s[p.pos]
		p.pos++
		start := p.pos
		for p.pos < len(p.s) && p.s[p.pos] != quote {
			p.pos++
		}
		if p.pos >= len(p.s) {
			return "", &exprError{"unterminated string literal"}
		}
		lit := p.s[start:p.pos]
		p.pos++
		return lit, nil
	}
	start := p.pos
	for p.pos < len(p.s) && !unicode.IsSpace(rune(p.s[p.pos])) && p.s[p.pos] != ')' {
		p.pos++
	}
	if start == p.pos {
		return "", &exprError{"expected literal value"}
	}
	return p.s[start:p.pos], nil
}

type exprError struct{ msg string }

func (e *exprError) Error() string { return e.msg }

// compareExpr evaluates a single comparison against the event fields.
func compareExpr(field, op, lit string, ev domain.Event) (bool, error) {
	switch field {
	case "metric":
		return stringCompare(op, ev.Metric, lit), nil
	case "device":
		return stringCompare(op, ev.DeviceID, lit), nil
	case "unit":
		return stringCompare(op, ev.Unit, lit), nil
	case "value":
		v, err := strconv.ParseFloat(lit, 64)
		if err != nil {
			return false, &exprError{"value comparison requires numeric literal"}
		}
		return valueCompare(op, ev.Value, v), nil
	}
	return false, &exprError{"unknown field " + field}
}

func stringCompare(op, got, want string) bool {
	switch op {
	case "==":
		return got == want
	case "!=":
		return got != want
	case "~":
		ok, err := path.Match(want, got)
		return err == nil && ok
	}
	return false
}

func valueCompare(op string, got, want float64) bool {
	switch op {
	case "==":
		return got == want
	case "!=":
		return got != want
	case "~":
		// For numeric fields, '~' is treated as equality within a tiny
		// epsilon to tolerate float representation differences.
		diff := got - want
		if diff < 0 {
			diff = -diff
		}
		return diff < 1e-9
	case ">":
		return got > want
	case ">=":
		return got >= want
	case "<":
		return got < want
	case "<=":
		return got <= want
	}
	return false
}
