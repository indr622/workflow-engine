package domain

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
	"github.com/indr622/workflow-engine/libs/go/feel"
)

// FeelEvaluator mengimplementasikan ExpressionEvaluator di atas kernel libs/go/feel (subset FEEL
// Fase 2, ADR-0013). Ekspresi yang sudah dikompilasi di-cache per teks sumber.
type FeelEvaluator struct {
	cache sync.Map // string → *feel.Expression
}

// NewFeelEvaluator membuat evaluator FEEL.
func NewFeelEvaluator() *FeelEvaluator { return &FeelEvaluator{} }

var _ ExpressionEvaluator = (*FeelEvaluator)(nil)

func (e *FeelEvaluator) compile(expr bpmn.Expression) (*feel.Expression, error) {
	src, _ := feel.StripPrefix(expr.Raw)
	if c, ok := e.cache.Load(src); ok {
		return c.(*feel.Expression), nil
	}
	c, err := feel.Compile(src)
	if err != nil {
		return nil, err
	}
	e.cache.Store(src, c)
	return c, nil
}

func (e *FeelEvaluator) evaluate(expr bpmn.Expression, vars VariableLookup) (feel.Value, error) {
	c, err := e.compile(expr)
	if err != nil {
		return feel.Value{}, err
	}
	return c.Evaluate(lookupContext{vars: vars})
}

// EvaluateBool mengevaluasi kondisi; hasil selain boolean (termasuk null) adalah error.
func (e *FeelEvaluator) EvaluateBool(expr bpmn.Expression, vars VariableLookup) (bool, error) {
	if !expr.IsFEEL {
		return false, fmt.Errorf("condition %q is not a FEEL expression (expected a leading '=')", expr.Raw)
	}
	v, err := e.evaluate(expr, vars)
	if err != nil {
		return false, err
	}
	b, ok := v.Bool()
	if !ok {
		return false, fmt.Errorf("expected a boolean but got %s", describe(v))
	}
	return b, nil
}

// EvaluateString mengembalikan literal apa adanya, atau hasil FEEL yang harus berupa string.
func (e *FeelEvaluator) EvaluateString(expr bpmn.Expression, vars VariableLookup) (string, error) {
	if !expr.IsFEEL {
		return strings.TrimSpace(expr.Raw), nil
	}
	v, err := e.evaluate(expr, vars)
	if err != nil {
		return "", err
	}
	s, ok := v.String()
	if !ok {
		return "", fmt.Errorf("expected a string but got %s", describe(v))
	}
	return s, nil
}

// EvaluateInt mem-parse literal bilangan bulat, atau hasil FEEL yang harus bilangan bulat.
func (e *FeelEvaluator) EvaluateInt(expr bpmn.Expression, vars VariableLookup) (int64, error) {
	if !expr.IsFEEL {
		n, err := strconv.ParseInt(strings.TrimSpace(expr.Raw), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("expected an integer but got %q", expr.Raw)
		}
		return n, nil
	}
	v, err := e.evaluate(expr, vars)
	if err != nil {
		return 0, err
	}
	n, ok := v.Int64()
	if !ok {
		return 0, fmt.Errorf("expected an integer but got %s", describe(v))
	}
	return n, nil
}

// EvaluateStringList memisah literal dengan koma (spasi di tepi dibuang, item kosong dilewati),
// atau mengevaluasi FEEL yang harus menghasilkan list string.
func (e *FeelEvaluator) EvaluateStringList(expr bpmn.Expression, vars VariableLookup) ([]string, error) {
	if !expr.IsFEEL {
		var out []string
		for _, part := range strings.Split(expr.Raw, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
		return out, nil
	}
	v, err := e.evaluate(expr, vars)
	if err != nil {
		return nil, err
	}
	items, ok := v.Items()
	if !ok {
		return nil, fmt.Errorf("expected a list of strings but got %s", describe(v))
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		s, ok := it.String()
		if !ok {
			return nil, fmt.Errorf("expected a list of strings but an item is %s", describe(it))
		}
		out = append(out, s)
	}
	return out, nil
}

func describe(v feel.Value) string {
	if v.IsNull() {
		return "null"
	}
	return v.Kind().String()
}

// lookupContext membaca variabel dari state secara malas: hanya variabel yang dirujuk ekspresi
// yang di-decode dari JSON.
type lookupContext struct {
	vars VariableLookup
}

func (c lookupContext) Lookup(name string) (feel.Value, bool) {
	if c.vars == nil {
		return feel.Value{}, false
	}
	raw, ok := c.vars.Lookup(name)
	if !ok {
		return feel.Value{}, false
	}
	v, err := feel.FromJSON(raw)
	if err != nil {
		// Nilai tersimpan selalu JSON valid; bila tidak, perlakukan sebagai variabel tidak ada.
		return feel.Value{}, false
	}
	return v, true
}
