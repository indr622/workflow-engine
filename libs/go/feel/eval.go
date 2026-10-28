package feel

// Evaluate menjalankan ekspresi terhadap konteks variabel (nil berarti tanpa variabel). Hasil null
// adalah nilai, bukan error; error dicadangkan untuk kegagalan evaluasi sungguhan, yang tidak ada
// di subset ini.
func (e *Expression) Evaluate(ctx Context) (Value, error) {
	return e.root.eval(ctx), nil
}

func (n literalNode) eval(Context) Value { return n.v }

func (n pathNode) eval(ctx Context) Value {
	if ctx == nil {
		return Value{}
	}
	v, ok := ctx.Lookup(n.names[0])
	if !ok {
		return Value{}
	}
	for _, name := range n.names[1:] {
		v = selectField(v, name)
	}
	return v
}

// selectField menerapkan path FEEL: field context (tidak ada → null); pada list, path diproyeksikan
// ke setiap elemen (semantik path expression FEEL); pada nilai lain → null.
func selectField(v Value, name string) Value {
	switch v.kind {
	case KindContext:
		return v.fields[name]
	case KindList:
		// verify: Camunda 8.8 — proyeksi path pada list dan perlakuan elemen non-context (di sini null).
		out := make([]Value, len(v.items))
		for i, it := range v.items {
			if it.kind == KindContext {
				out[i] = it.fields[name]
			}
		}
		return Value{kind: KindList, items: out}
	}
	return Value{}
}

func (n notNode) eval(ctx Context) Value {
	v := n.x.eval(ctx)
	if v.kind != KindBool {
		return Value{}
	}
	return NewBool(!v.b)
}

// and/or: logika tiga nilai FEEL; operand non-boolean diperlakukan sebagai null.
func (n andNode) eval(ctx Context) Value {
	sawNull := false
	for _, x := range n.xs {
		v := x.eval(ctx)
		if v.kind != KindBool {
			sawNull = true
			continue
		}
		if !v.b {
			return NewBool(false)
		}
	}
	if sawNull {
		return Value{}
	}
	return NewBool(true)
}

func (n orNode) eval(ctx Context) Value {
	sawNull := false
	for _, x := range n.xs {
		v := x.eval(ctx)
		if v.kind != KindBool {
			sawNull = true
			continue
		}
		if v.b {
			return NewBool(true)
		}
	}
	if sawNull {
		return Value{}
	}
	return NewBool(false)
}

func (n compareNode) eval(ctx Context) Value {
	l, r := n.l.eval(ctx), n.r.eval(ctx)
	switch n.op {
	case tokEq:
		return equal(l, r)
	case tokNeq:
		eq := equal(l, r)
		if eq.kind != KindBool {
			return eq
		}
		return NewBool(!eq.b)
	}
	c, ok := order(l, r)
	if !ok {
		return Value{}
	}
	switch n.op {
	case tokLt:
		return NewBool(c < 0)
	case tokLe:
		return NewBool(c <= 0)
	case tokGt:
		return NewBool(c > 0)
	default: // tokGe
		return NewBool(c >= 0)
	}
}

// order membandingkan number/number dan string/string (urutan code point Unicode). Tipe lain,
// campuran, atau null tidak terurut.
func order(l, r Value) (int, bool) {
	switch {
	case l.kind == KindNumber && r.kind == KindNumber:
		return l.n.Cmp(r.n), true
	case l.kind == KindString && r.kind == KindString:
		// Perbandingan byte UTF-8 setara dengan urutan code point.
		// verify: Camunda 8.8 — feel-scala (Java) membandingkan unit UTF-16, berbeda untuk
		// karakter di atas U+FFFF terhadap U+E000..U+FFFF.
		switch {
		case l.s < r.s:
			return -1, true
		case l.s > r.s:
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// equal mengimplementasikan "=" FEEL: null = null → true, x = null → false, tipe berbeda → null,
// context/list dibandingkan dalam (deep).
func equal(l, r Value) Value {
	if l.kind == KindNull || r.kind == KindNull {
		return NewBool(l.kind == r.kind)
	}
	if l.kind != r.kind {
		return Value{}
	}
	switch l.kind {
	case KindBool:
		return NewBool(l.b == r.b)
	case KindNumber:
		return NewBool(l.n.Cmp(r.n) == 0)
	case KindString:
		return NewBool(l.s == r.s)
	case KindContext:
		// verify: Camunda 8.8 — kesetaraan context: kunci sama dan setiap field setara.
		if len(l.fields) != len(r.fields) {
			return NewBool(false)
		}
		sawNull := false
		for k, lv := range l.fields {
			rv, ok := r.fields[k]
			if !ok {
				return NewBool(false)
			}
			if res, done := combineEqual(equal(lv, rv), &sawNull); done {
				return res
			}
		}
		return finishEqual(sawNull)
	case KindList:
		// verify: Camunda 8.8 — kesetaraan list: panjang sama dan elemen setara berurutan.
		if len(l.items) != len(r.items) {
			return NewBool(false)
		}
		sawNull := false
		for i := range l.items {
			if res, done := combineEqual(equal(l.items[i], r.items[i]), &sawNull); done {
				return res
			}
		}
		return finishEqual(sawNull)
	}
	return Value{}
}

// combineEqual: satu pasangan tidak setara → false; pasangan tak terbandingkan (null) dicatat.
func combineEqual(eq Value, sawNull *bool) (Value, bool) {
	if eq.kind != KindBool {
		*sawNull = true
		return Value{}, false
	}
	if !eq.b {
		return NewBool(false), true
	}
	return Value{}, false
}

func finishEqual(sawNull bool) Value {
	if sawNull {
		return Value{}
	}
	return NewBool(true)
}
