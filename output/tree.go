package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/heyllave/query/ast"
	"github.com/heyllave/query/token"
)

// treeNode is the intermediate representation produced by treeVisitor.
// The visitor builds a treeNode tree from the AST; the renderer draws
// it with box-drawing characters.
type treeNode struct {
	Label    string
	Position *token.Position
	Children []*treeNode
}

// treeFormatter writes the AST as a tree with box-drawing characters.
type treeFormatter struct{}

func (f *treeFormatter) Format(w io.Writer, expr ast.Expression, opts Options) error {
	v := &treeVisitor{positions: opts.Positions}
	root := ast.Visit[*treeNode](v, expr)
	var buf strings.Builder
	// Root node: print label without connector, then render children
	buf.WriteString(root.Label)
	writePosition(&buf, root.Position)
	buf.WriteByte('\n')
	for i, child := range root.Children {
		renderNode(&buf, child, "", i == len(root.Children)-1)
	}
	_, err := io.WriteString(w, buf.String())
	return err
}

// renderNode recursively renders a treeNode with box-drawing connectors.
func renderNode(buf *strings.Builder, node *treeNode, prefix string, isLast bool) {
	connector := "├── "
	childPrefix := prefix + "│   "
	if isLast {
		connector = "└── "
		childPrefix = prefix + "    "
	}

	buf.WriteString(prefix + connector + node.Label)
	writePosition(buf, node.Position)
	buf.WriteByte('\n')

	for i, child := range node.Children {
		renderNode(buf, child, childPrefix, i == len(node.Children)-1)
	}
}

func writePosition(buf *strings.Builder, pos *token.Position) {
	if pos != nil {
		fmt.Fprintf(buf, " [%d:%d]", pos.Offset, pos.Length)
	}
}

// treeVisitor implements ast.Visitor[*treeNode].
type treeVisitor struct {
	positions bool
}

func (v *treeVisitor) VisitBinary(e *ast.BinaryExpr) *treeNode {
	label := "AndExpr"
	if e.Op == token.Or {
		label = "OrExpr"
	}
	return &treeNode{
		Label:    label,
		Position: v.pos(e.Position),
		Children: []*treeNode{
			ast.Visit[*treeNode](v, e.Left),
			ast.Visit[*treeNode](v, e.Right),
		},
	}
}

func (v *treeVisitor) VisitUnary(e *ast.UnaryExpr) *treeNode {
	return &treeNode{
		Label:    "NotExpr",
		Position: v.pos(e.Position),
		Children: []*treeNode{ast.Visit[*treeNode](v, e.Expr)},
	}
}

func (v *treeVisitor) VisitQualifier(e *ast.QualifierExpr) *treeNode {
	label := fmt.Sprintf("QualifierExpr (%s)", token.OperatorSymbol(e.Operator))
	node := &treeNode{
		Label:    label,
		Position: v.pos(e.Position),
	}

	if e.HasFieldFunc() {
		node.Children = append(node.Children, &treeNode{
			Label:    fmt.Sprintf("FieldFunc: %s(%s)", e.FieldFunc.Name, e.Field.String()),
			Position: v.pos(e.FieldFunc.Position),
		})
	} else {
		node.Children = append(node.Children, &treeNode{
			Label:    fmt.Sprintf("Field: %s", e.Field.String()),
			Position: v.pos(e.Position),
		})
	}

	node.Children = append(node.Children, &treeNode{
		Label:    fmt.Sprintf("Value: %s (%s)", e.Value.Raw, e.Value.Type),
		Position: v.pos(e.Position),
	})

	if e.IsRange() {
		node.Children = append(node.Children, &treeNode{
			Label:    fmt.Sprintf("EndValue: %s (%s)", e.EndValue.Raw, e.EndValue.Type),
			Position: v.pos(e.Position),
		})
	}

	return node
}

func (v *treeVisitor) VisitPresence(e *ast.PresenceExpr) *treeNode {
	return &treeNode{
		Label:    "PresenceExpr",
		Position: v.pos(e.Position),
		Children: []*treeNode{
			{
				Label:    fmt.Sprintf("Field: %s", e.Field.String()),
				Position: v.pos(e.Position),
			},
		},
	}
}

func (v *treeVisitor) VisitGroup(e *ast.GroupExpr) *treeNode {
	return &treeNode{
		Label:    "GroupExpr",
		Position: v.pos(e.Position),
		Children: []*treeNode{ast.Visit[*treeNode](v, e.Expr)},
	}
}

func (v *treeVisitor) VisitSelector(e *ast.SelectorExpr) *treeNode {
	label := "SelectorExpr (@(...))"
	if e.Selector != "" {
		label = fmt.Sprintf("SelectorExpr (@%s)", e.Selector)
	}
	node := &treeNode{
		Label:    label,
		Position: v.pos(e.Position),
		Children: []*treeNode{ast.Visit[*treeNode](v, e.Base)},
	}
	if e.Inner != nil {
		node.Children = append(node.Children, ast.Visit[*treeNode](v, e.Inner))
	}
	return node
}

func (v *treeVisitor) VisitFuncCall(e *ast.FuncCallExpr) *treeNode {
	node := &treeNode{
		Label:    fmt.Sprintf("FuncCallExpr (%s)", e.Name),
		Position: v.pos(e.Position),
	}
	for _, arg := range e.Args {
		if arg.Call != nil {
			node.Children = append(node.Children, ast.Visit[*treeNode](v, arg.Call))
		} else {
			node.Children = append(node.Children, &treeNode{
				Label:    fmt.Sprintf("Arg: %s", arg.String()),
				Position: v.pos(e.Position),
			})
		}
	}
	return node
}

func (v *treeVisitor) pos(p token.Position) *token.Position {
	if v.positions {
		return &p
	}
	return nil
}
