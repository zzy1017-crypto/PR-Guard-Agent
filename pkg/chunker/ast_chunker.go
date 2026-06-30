package chunker

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strings"

	projecthash "pr-guard-agent/pkg/hash"
)

type ASTChunk struct {
	FilePath        string
	SymbolName      string
	SymbolType      string
	StartLine       int
	EndLine         int
	ChunkText       string
	ContentHash     string
	CodeVersionHash string
}

func ParserFileToChunks(filePath string, relativePath string, codeVersionHash string) ([]ASTChunk, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read go file failed: %w", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse go file failed: %w", err)
	}

	chunks := make([]ASTChunk, 0)
	for _, decl := range file.Decls {
		switch node := decl.(type) {
		case *ast.FuncDecl:
			symbolType := "function"
			symbolName := node.Name.Name
			if node.Recv != nil && len(node.Recv.List) > 0 {
				symbolType = "method"
				symbolName = receiverTypeName(fset, node.Recv.List[0].Type) + "." + node.Name.Name
			}

			chunk, err := buildASTChunk(fset, source, relativePath, symbolName, symbolType, node, codeVersionHash)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, chunk)
		case *ast.GenDecl:
			genChunks, err := parseGenDecl(fset, source, relativePath, node, codeVersionHash)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, genChunks...)
		}
	}

	return chunks, nil
}

func ParserGoFileToChunks(filePath string, relativePath string, codeVersionHash string) ([]ASTChunk, error) {
	return ParserFileToChunks(filePath, relativePath, codeVersionHash)
}

func ParserGoFileToChunk(filePath string, relativePath string, codeVersionHash string) ([]ASTChunk, error) {
	return ParserFileToChunks(filePath, relativePath, codeVersionHash)
}

func parseGenDecl(fset *token.FileSet, source []byte, relativePath string, node *ast.GenDecl, codeVersionHash string) ([]ASTChunk, error) {
	chunks := make([]ASTChunk, 0)

	for _, spec := range node.Specs {
		switch node.Tok {
		case token.TYPE:
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			symbolType := ""
			switch typeSpec.Type.(type) {
			case *ast.StructType:
				symbolType = "struct"
			case *ast.InterfaceType:
				symbolType = "interface"
			default:
				continue
			}

			chunk, err := buildASTChunk(fset, source, relativePath, typeSpec.Name.Name, symbolType, nodeForSpec(node, spec), codeVersionHash)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, chunk)
		case token.CONST:
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range valueSpec.Names {
				chunk, err := buildASTChunk(fset, source, relativePath, name.Name, "const", nodeForSpec(node, spec), codeVersionHash)
				if err != nil {
					return nil, err
				}
				chunks = append(chunks, chunk)
			}
		case token.VAR:
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range valueSpec.Names {
				chunk, err := buildASTChunk(fset, source, relativePath, name.Name, "var", nodeForSpec(node, spec), codeVersionHash)
				if err != nil {
					return nil, err
				}
				chunks = append(chunks, chunk)
			}
		}
	}

	return chunks, nil
}

func nodeForSpec(genDecl *ast.GenDecl, spec ast.Spec) ast.Node {
	if len(genDecl.Specs) == 1 {
		return genDecl
	}
	return spec
}

func buildASTChunk(fset *token.FileSet, source []byte, relativePath string, symbolName string, symbolType string, node ast.Node, codeVersionHash string) (ASTChunk, error) {
	start := fset.Position(node.Pos())
	end := fset.Position(node.End())
	if start.Offset < 0 || end.Offset > len(source) || start.Offset >= end.Offset {
		return ASTChunk{}, fmt.Errorf("invalid ast node range for %s: %d-%d", relativePath, start.Offset, end.Offset)
	}

	chunkText := string(source[start.Offset:end.Offset])
	return ASTChunk{
		FilePath:        relativePath,
		SymbolName:      symbolName,
		SymbolType:      symbolType,
		StartLine:       start.Line,
		EndLine:         end.Line,
		ChunkText:       chunkText,
		ContentHash:     projecthash.SHA256String(chunkText),
		CodeVersionHash: codeVersionHash,
	}, nil
}

func receiverTypeName(fset *token.FileSet, expr ast.Expr) string {
	for {
		switch t := expr.(type) {
		case *ast.StarExpr:
			expr = t.X
		case *ast.ParenExpr:
			expr = t.X
		default:
			var buf bytes.Buffer
			if err := printer.Fprint(&buf, fset, expr); err != nil {
				return ""
			}
			return strings.TrimSpace(buf.String())
		}
	}
}
