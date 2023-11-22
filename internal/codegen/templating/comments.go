package templating

import (
	"bytes"
	"embed"
	"text/template"

	"google.golang.org/protobuf/compiler/protogen"
)

// CommentRenderer renders go templates in protobuf comments. This is useful
// as the first generator in sequence, allowing subsequent generators to
// use the rendered comment text in their own code generation.
//
// Only certain comments are checked for templates. Additional comment nodes
// can be inspected as-needed by modifying the Generate method.
type CommentRenderer struct{}

func (CommentRenderer) Name() string {
	return "templating-comment-generator"
}

func (CommentRenderer) Generate(gen *protogen.Plugin) error {
	for _, file := range gen.Files {
		if !file.Generate {
			continue
		}
		for _, svc := range file.Services {
			for _, mtd := range svc.Methods {
				applyRPCTemplates(&mtd.Comments)
			}
		}
	}
	return nil
}

//go:embed templates
var templatesFS embed.FS

var rpcTemplates = template.Must(template.New("comments").ParseFS(templatesFS, "templates/rpc.tmpl"))

func applyRPCTemplates(comments *protogen.CommentSet) error {
	tmpl := template.Must(rpcTemplates.New("").Parse(string(comments.Leading)))

	var leadingCommentsBuf bytes.Buffer
	if err := tmpl.Execute(&leadingCommentsBuf, nil); err != nil {
		return err
	}
	comments.Leading = protogen.Comments(leadingCommentsBuf.String())
	return nil
}
