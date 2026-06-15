package tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	sdkImportPath = "github.com/memohai/twilight-ai/sdk"
	mcpImportPath = "github.com/memohai/memoh/internal/mcp"
)

func TestBuiltInToolNamesAreUnique(t *testing.T) {
	t.Parallel()

	constants, mapKeys := toolNamesGoConstants(t)
	seen := make(map[string]ToolName)
	for name := range builtInToolNames {
		raw := name.String()
		if raw == "" {
			t.Fatal("built-in tool name must not be empty")
		}
		if prev, ok := seen[raw]; ok {
			t.Fatalf("duplicate built-in tool name %q for %s and %s", raw, prev, name)
		}
		seen[raw] = name
	}
	for name := range constants {
		if _, ok := mapKeys[name]; !ok {
			t.Fatalf("tool constant %s must be listed in builtInToolNames", name)
		}
	}
	for name := range mapKeys {
		if _, ok := constants[name]; !ok {
			t.Fatalf("builtInToolNames contains %s, but names.go does not declare that tool constant", name)
		}
	}
}

func TestBuiltInToolNamesUseConstantsInProviders(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob tools files: %v", err)
	}
	constants, _ := toolNamesGoConstants(t)
	hasStringPattern := regexp.MustCompile(`available\.Has\("`)

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || file == "names.go" {
			continue
		}
		src := readGoSource(t, file)
		text := string(src)
		if hasStringPattern.MatchString(text) {
			t.Fatalf("%s uses available.Has with a string literal; use a ToolName constant", file)
		}
		checkGoFileForToolNames(t, file, src, constants)
	}
}

func TestMemoryAdapterMCPToolNamesUseConstants(t *testing.T) {
	t.Parallel()

	files := globGoFiles(t, "../../memory/adapters/*.go", "../../memory/adapters/*/*.go")
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src := readGoSource(t, file)
		checkGoFileForMCPToolNames(t, file, src)
	}
}

func readGoSource(t *testing.T, file string) []byte {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	return src
}

func globGoFiles(t *testing.T, patterns ...string) []string {
	t.Helper()
	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		files = append(files, matches...)
	}
	return files
}

func checkGoFileForToolNames(t *testing.T, file string, src []byte, constants map[string]struct{}) {
	t.Helper()

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	sdkAliases := importAliases(parsed, sdkImportPath, "sdk")
	ast.Inspect(parsed, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if ok {
			if isSDKToolComposite(lit.Type, sdkAliases) {
				checkSDKToolName(t, fset, file, lit, constants, allowDynamicSDKDescriptorName(file))
			}
			if isSDKToolSlice(lit.Type, sdkAliases) {
				for _, elt := range lit.Elts {
					child, ok := elt.(*ast.CompositeLit)
					if ok && child.Type == nil {
						checkSDKToolName(t, fset, file, child, constants, allowDynamicSDKDescriptorName(file))
					}
				}
			}
		}
		if call, ok := n.(*ast.CallExpr); ok && isSDKNewToolCall(call, sdkAliases) {
			if err := checkSDKNewToolNameValue(call, constants); err != nil {
				t.Fatalf("%s:%d registers sdk.NewTool with invalid name: %v", file, fset.Position(call.Pos()).Line, err)
			}
		}
		return true
	})
}

func allowDynamicSDKDescriptorName(file string) bool {
	switch filepath.Base(file) {
	case "federation.go", "memory.go":
		return true
	default:
		return false
	}
}

func checkGoFileForMCPToolNames(t *testing.T, file string, src []byte) {
	t.Helper()

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	mcpAliases := importAliases(parsed, mcpImportPath, "mcp")
	ast.Inspect(parsed, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if isMCPToolDescriptorComposite(lit.Type, mcpAliases) {
			checkMCPToolDescriptorName(t, fset, file, lit)
		}
		if isMCPToolDescriptorSlice(lit.Type, mcpAliases) {
			for _, elt := range lit.Elts {
				child, ok := elt.(*ast.CompositeLit)
				if ok && child.Type == nil {
					checkMCPToolDescriptorName(t, fset, file, child)
				}
			}
		}
		return true
	})
}

func importAliases(file *ast.File, importPath, defaultName string) map[string]struct{} {
	aliases := map[string]struct{}{}
	for _, spec := range file.Imports {
		raw, err := strconv.Unquote(spec.Path.Value)
		if err != nil || raw != importPath {
			continue
		}
		if spec.Name == nil {
			aliases[defaultName] = struct{}{}
			continue
		}
		if spec.Name.Name != "" && spec.Name.Name != "." && spec.Name.Name != "_" {
			aliases[spec.Name.Name] = struct{}{}
		}
	}
	return aliases
}

func toolNamesGoConstants(t *testing.T) (map[string]struct{}, map[string]struct{}) {
	t.Helper()

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "names.go", nil, 0)
	if err != nil {
		t.Fatalf("parse names.go: %v", err)
	}
	constants := map[string]struct{}{}
	mapKeys := map[string]struct{}{}
	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		switch gen.Tok {
		case token.CONST:
			for _, spec := range gen.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				for _, name := range valueSpec.Names {
					if strings.HasPrefix(name.Name, "Tool") {
						constants[name.Name] = struct{}{}
					}
				}
			}
		case token.VAR:
			for _, spec := range gen.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				if len(valueSpec.Names) != 1 || valueSpec.Names[0].Name != "builtInToolNames" || len(valueSpec.Values) != 1 {
					continue
				}
				lit, ok := valueSpec.Values[0].(*ast.CompositeLit)
				if !ok {
					t.Fatalf("builtInToolNames must be a map literal")
				}
				for _, elt := range lit.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						t.Fatalf("builtInToolNames entries must use keyed tool constants")
					}
					name, ok := kv.Key.(*ast.Ident)
					if !ok {
						t.Fatalf("builtInToolNames key at %s must be a tool constant", fset.Position(kv.Key.Pos()))
					}
					mapKeys[name.Name] = struct{}{}
				}
			}
		}
	}
	return constants, mapKeys
}

func isSDKToolSlice(expr ast.Expr, sdkAliases map[string]struct{}) bool {
	array, ok := expr.(*ast.ArrayType)
	if !ok {
		return false
	}
	return isSDKToolComposite(array.Elt, sdkAliases)
}

func isSDKToolComposite(expr ast.Expr, sdkAliases map[string]struct{}) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Tool" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = sdkAliases[pkg.Name]
	return ok
}

func isMCPToolDescriptorSlice(expr ast.Expr, mcpAliases map[string]struct{}) bool {
	array, ok := expr.(*ast.ArrayType)
	if !ok {
		return false
	}
	return isMCPToolDescriptorComposite(array.Elt, mcpAliases)
}

func isMCPToolDescriptorComposite(expr ast.Expr, mcpAliases map[string]struct{}) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "ToolDescriptor" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = mcpAliases[pkg.Name]
	return ok
}

func checkSDKToolName(t *testing.T, fset *token.FileSet, file string, lit *ast.CompositeLit, constants map[string]struct{}, allowDynamicDescriptorName bool) {
	t.Helper()

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Name" {
			continue
		}
		if err := checkSDKToolNameValue(kv.Value, constants, allowDynamicDescriptorName); err != nil {
			t.Fatalf("%s:%d registers sdk.Tool with invalid Name: %v", file, fset.Position(kv.Value.Pos()).Line, err)
		}
	}
}

func checkSDKToolNameValue(expr ast.Expr, constants map[string]struct{}, allowDynamicDescriptorName bool) error {
	switch value := expr.(type) {
	case *ast.BasicLit:
		if value.Kind == token.STRING {
			raw, _ := strconv.Unquote(value.Value)
			return &toolNameError{"string literal " + strconv.Quote(raw) + "; use a central ToolName constant or dynamic descriptor name"}
		}
	case *ast.CallExpr:
		sel, ok := value.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "String" {
			return &toolNameError{"call expression is not ToolName.String()"}
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return &toolNameError{"ToolName.String() receiver must be a central tool constant"}
		}
		if _, ok := constants[ident.Name]; !ok {
			return &toolNameError{ident.Name + ".String() is not declared in names.go and listed in builtInToolNames"}
		}
		return nil
	case *ast.SelectorExpr:
		if allowDynamicDescriptorName && selectorIsDynamicDescriptorName(value) {
			return nil
		}
		return &toolNameError{"selector expression is not an allowed dynamic descriptor name"}
	}
	return &toolNameError{"use a central ToolName constant or dynamic descriptor name"}
}

func selectorIsDynamicDescriptorName(value *ast.SelectorExpr) bool {
	ident, ok := value.X.(*ast.Ident)
	return ok && ident.Name == "desc" && value.Sel.Name == "Name"
}

func checkMCPToolDescriptorName(t *testing.T, fset *token.FileSet, file string, lit *ast.CompositeLit) {
	t.Helper()

	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Name" {
			continue
		}
		if err := checkMCPToolDescriptorNameValue(kv.Value); err != nil {
			t.Fatalf("%s:%d registers mcp.ToolDescriptor with invalid Name: %v", file, fset.Position(kv.Value.Pos()).Line, err)
		}
	}
}

func checkMCPToolDescriptorNameValue(expr ast.Expr) error {
	switch value := expr.(type) {
	case *ast.BasicLit:
		if value.Kind == token.STRING {
			raw, _ := strconv.Unquote(value.Value)
			return &toolNameError{"string literal " + strconv.Quote(raw) + "; use the memory adapters ToolSearchMemory constant"}
		}
	case *ast.SelectorExpr:
		pkg, ok := value.X.(*ast.Ident)
		if ok && (pkg.Name == "adapters" || pkg.Name == "memprovider") && value.Sel.Name == "ToolSearchMemory" {
			return nil
		}
		return &toolNameError{"selector expression is not an allowed memory tool constant"}
	}
	return &toolNameError{"use the memory adapters ToolSearchMemory constant"}
}

func isSDKNewToolCall(call *ast.CallExpr, sdkAliases map[string]struct{}) bool {
	return isSDKNewToolFun(call.Fun, sdkAliases)
}

func isSDKNewToolFun(expr ast.Expr, sdkAliases map[string]struct{}) bool {
	switch value := expr.(type) {
	case *ast.SelectorExpr:
		return selectorIsSDKNewTool(value, sdkAliases)
	case *ast.IndexExpr:
		return isSDKNewToolFun(value.X, sdkAliases)
	case *ast.IndexListExpr:
		return isSDKNewToolFun(value.X, sdkAliases)
	default:
		return false
	}
}

func selectorIsSDKNewTool(sel *ast.SelectorExpr, sdkAliases map[string]struct{}) bool {
	if sel.Sel.Name != "NewTool" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = sdkAliases[pkg.Name]
	return ok
}

func checkSDKNewToolNameValue(call *ast.CallExpr, constants map[string]struct{}) error {
	if len(call.Args) == 0 {
		return &toolNameError{"sdk.NewTool missing name argument"}
	}
	return checkSDKToolNameValue(call.Args[0], constants, false)
}

func TestBuiltInToolNamesRejectRawStringInSDKNewTool(t *testing.T) {
	t.Parallel()

	expr, err := parser.ParseExpr(`sdk.NewTool[map[string]any]("raw_tool", "desc", fn)`)
	if err != nil {
		t.Fatalf("parse expression: %v", err)
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected call expression")
	}
	if err := checkSDKNewToolNameValue(call, nil); err == nil {
		t.Fatal("sdk.NewTool with a raw string name must be rejected")
	}

	expr, err = parser.ParseExpr(`sdk.NewTool[map[string]any](ToolRead.String(), "desc", fn)`)
	if err != nil {
		t.Fatalf("parse expression: %v", err)
	}
	call, ok = expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected call expression")
	}
	if err := checkSDKNewToolNameValue(call, map[string]struct{}{"ToolRead": {}}); err != nil {
		t.Fatalf("sdk.NewTool with ToolName.String() should pass: %v", err)
	}
}

func TestBuiltInToolNamesGuardRecognizesImportAliases(t *testing.T) {
	t.Parallel()

	src := []byte(`package tools

import (
	twilight "github.com/memohai/twilight-ai/sdk"
	mcpgw "github.com/memohai/memoh/internal/mcp"
)

var _ = []twilight.Tool{{Name: "raw_tool"}}
var _ = twilight.NewTool[map[string]any]("raw_tool", "desc", nil)
var _ = []mcpgw.ToolDescriptor{{Name: "search_memory"}}
`)
	parsed, err := parser.ParseFile(token.NewFileSet(), "alias.go", src, 0)
	if err != nil {
		t.Fatalf("parse alias file: %v", err)
	}
	sdkAliases := importAliases(parsed, sdkImportPath, "sdk")
	mcpAliases := importAliases(parsed, mcpImportPath, "mcp")
	if _, ok := sdkAliases["sdk"]; ok {
		t.Fatal("explicit SDK alias should not also register the default sdk name")
	}
	if _, ok := mcpAliases["mcp"]; ok {
		t.Fatal("explicit MCP alias should not also register the default mcp name")
	}
	var sawSDKToolAlias, sawSDKNewToolAlias, sawMCPDescriptorAlias bool
	ast.Inspect(parsed, func(n ast.Node) bool {
		if lit, ok := n.(*ast.CompositeLit); ok {
			if isSDKToolSlice(lit.Type, sdkAliases) {
				sawSDKToolAlias = true
				child := lit.Elts[0].(*ast.CompositeLit)
				if err := firstNameValueError(child, checkSDKToolNameValue, map[string]struct{}{"ToolRead": {}}); err == nil {
					t.Fatal("sdk.Tool alias with raw string name must be rejected")
				}
			}
			if isMCPToolDescriptorSlice(lit.Type, mcpAliases) {
				sawMCPDescriptorAlias = true
				child := lit.Elts[0].(*ast.CompositeLit)
				if err := firstMCPNameValueError(child); err == nil {
					t.Fatal("mcp.ToolDescriptor alias with raw string name must be rejected")
				}
			}
		}
		if call, ok := n.(*ast.CallExpr); ok && isSDKNewToolCall(call, sdkAliases) {
			sawSDKNewToolAlias = true
			if err := checkSDKNewToolNameValue(call, map[string]struct{}{"ToolRead": {}}); err == nil {
				t.Fatal("sdk.NewTool alias with raw string name must be rejected")
			}
		}
		return true
	})
	if !sawSDKToolAlias || !sawSDKNewToolAlias || !sawMCPDescriptorAlias {
		t.Fatalf("alias guard did not observe all aliased tool declarations: sdk.Tool=%v sdk.NewTool=%v mcp.ToolDescriptor=%v", sawSDKToolAlias, sawSDKNewToolAlias, sawMCPDescriptorAlias)
	}
}

func firstNameValueError(lit *ast.CompositeLit, check func(ast.Expr, map[string]struct{}, bool) error, constants map[string]struct{}) error {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if ok && key.Name == "Name" {
			return check(kv.Value, constants, false)
		}
	}
	return nil
}

func firstMCPNameValueError(lit *ast.CompositeLit) error {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if ok && key.Name == "Name" {
			return checkMCPToolDescriptorNameValue(kv.Value)
		}
	}
	return nil
}

type toolNameError struct {
	msg string
}

func (e *toolNameError) Error() string {
	return e.msg
}
