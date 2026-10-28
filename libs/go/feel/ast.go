package feel

// node adalah simpul AST yang sudah tervalidasi; eval tidak pernah gagal untuk subset ini.
type node interface {
	eval(ctx Context) Value
}

type literalNode struct{ v Value }

type pathNode struct{ names []string }

type notNode struct{ x node }

type andNode struct{ xs []node }

type orNode struct{ xs []node }

type compareNode struct {
	op   tokenKind
	l, r node
}
