package eval

import (
	"errors"
	"fmt"

	"github.com/heyllave/query/ast"
)

// ErrUnknownFunction is wrapped by the error [Compile] and [CompileValue]
// return when the query calls a function the registry does not hold.
var ErrUnknownFunction = errors.New("unknown function")

// ErrArity is wrapped by the error [Compile] and [CompileValue] return when a
// call has an argument count outside the function's declared [Arity].
var ErrArity = errors.New("wrong number of arguments")

// checkCalls verifies every function call in the expression, wherever it
// appears, names a registered function and satisfies its arity. A call that
// fails either check used to compile into a predicate that was silently false
// for every record; it is a compile error instead.
func checkCalls(expr ast.Expression, funcs FuncRegistry) error {
	var errs []error
	for _, fc := range ast.Calls(expr) {
		fn, ok := funcs.Get(fc.Name)
		if !ok {
			errs = append(errs, fmt.Errorf("position %d: %w %q", fc.Position.Offset, ErrUnknownFunction, fc.Name))
			continue
		}
		if err := fn.Arity.Check(fc.Name, len(fc.Args)); err != nil {
			errs = append(errs, fmt.Errorf("position %d: %w: %w", fc.Position.Offset, ErrArity, err))
		}
	}
	return errors.Join(errs...)
}
