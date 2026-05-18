package output

import (
	"encoding/json"
	"io"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// jsonNode is the JSON-serializable representation of an AST node.
type jsonNode struct {
	Type      string      `json:"type"`
	Position  *jsonPos    `json:"position,omitempty"`
	Op        string      `json:"op,omitempty"`
	Field     string      `json:"field,omitempty"`
	Value     string      `json:"value,omitempty"`
	ValueT    string      `json:"value_type,omitempty"`
	EndValue  string      `json:"end_value,omitempty"`
	EndValueT string      `json:"end_value_type,omitempty"`
	Selector  string      `json:"selector,omitempty"`
	FuncName  string      `json:"func_name,omitempty"`
	Args      []jsonArg   `json:"args,omitempty"`
	Children  []*jsonNode `json:"children,omitempty"`
}

type jsonPos struct {
	Offset int `json:"offset"`
	Length int `json:"length"`
}

type jsonArg struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// jsonFormatter writes the AST as indented JSON.
type jsonFormatter struct{}

func (f *jsonFormatter) Format(w io.Writer, expr ast.Expression, opts Options) error {
	v := &jsonVisitor{positions: opts.Positions}
	node := ast.Visit[*jsonNode](v, expr)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(node)
}

// jsonVisitor implements ast.Visitor[*jsonNode].
type jsonVisitor struct {
	positions bool
}

func (v *jsonVisitor) VisitBinary(e *ast.BinaryExpr) *jsonNode {
	opName := "AND"
	if e.Op == token.Or {
		opName = "OR"
	}
	node := &jsonNode{
		Type: "BinaryExpr",
		Op:   opName,
		Children: []*jsonNode{
			ast.Visit[*jsonNode](v, e.Left),
			ast.Visit[*jsonNode](v, e.Right),
		},
	}
	v.setPos(node, e.Position)
	return node
}

func (v *jsonVisitor) VisitUnary(e *ast.UnaryExpr) *jsonNode {
	node := &jsonNode{
		Type:     "UnaryExpr",
		Op:       "NOT",
		Children: []*jsonNode{ast.Visit[*jsonNode](v, e.Expr)},
	}
	v.setPos(node, e.Position)
	return node
}

func (v *jsonVisitor) VisitQualifier(e *ast.QualifierExpr) *jsonNode {
	node := &jsonNode{
		Type:   "QualifierExpr",
		Op:     token.OperatorSymbol(e.Operator),
		Field:  e.Field.String(),
		Value:  e.Value.Raw,
		ValueT: e.Value.Type.String(),
	}
	if e.HasFieldFunc() {
		node.FuncName = e.FieldFunc.Name
	}
	if e.IsRange() {
		node.EndValue = e.EndValue.Raw
		node.EndValueT = e.EndValue.Type.String()
	}
	v.setPos(node, e.Position)
	return node
}

func (v *jsonVisitor) VisitPresence(e *ast.PresenceExpr) *jsonNode {
	node := &jsonNode{
		Type:  "PresenceExpr",
		Field: e.Field.String(),
	}
	v.setPos(node, e.Position)
	return node
}

func (v *jsonVisitor) VisitGroup(e *ast.GroupExpr) *jsonNode {
	node := &jsonNode{
		Type:     "GroupExpr",
		Children: []*jsonNode{ast.Visit[*jsonNode](v, e.Expr)},
	}
	v.setPos(node, e.Position)
	return node
}

func (v *jsonVisitor) VisitSelector(e *ast.SelectorExpr) *jsonNode {
	node := &jsonNode{
		Type:     "SelectorExpr",
		Selector: e.Selector,
		Children: []*jsonNode{ast.Visit[*jsonNode](v, e.Base)},
	}
	if e.Inner != nil {
		node.Children = append(node.Children, ast.Visit[*jsonNode](v, e.Inner))
	}
	v.setPos(node, e.Position)
	return node
}

func (v *jsonVisitor) VisitFuncCall(e *ast.FuncCallExpr) *jsonNode {
	node := &jsonNode{
		Type:     "FuncCallExpr",
		FuncName: e.Name,
	}
	for _, arg := range e.Args {
		ja := jsonArg{}
		switch {
		case arg.Field != nil:
			ja.Type = "field"
			ja.Value = arg.Field.String()
		case arg.Value != nil:
			ja.Type = arg.Value.Type.String()
			ja.Value = arg.Value.Raw
		case arg.Call != nil:
			ja.Type = "call"
			ja.Value = arg.Call.Name
			node.Children = append(node.Children, ast.Visit[*jsonNode](v, arg.Call))
		}
		node.Args = append(node.Args, ja)
	}
	v.setPos(node, e.Position)
	return node
}

func (v *jsonVisitor) setPos(node *jsonNode, pos token.Position) {
	if v.positions {
		node.Position = &jsonPos{Offset: pos.Offset, Length: pos.Length}
	}
}
