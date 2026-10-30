package ubl

import (
	"context"
	_ "embed"
	"fmt"
	"sync"

	"github.com/furkanmeclis/go-ubltr/render"
	heliumxslt "github.com/furkanmeclis/go-ubltr/render/xslt/helium"
)

//go:embed invoice.xsl
var stylesheet []byte

var (
	htmlOnce sync.Once
	htmlR    *render.HTMLRenderer
	htmlErr  error
)

func htmlRenderer() (*render.HTMLRenderer, error) {
	htmlOnce.Do(func() {
		htmlR, htmlErr = render.NewHTML(heliumxslt.New(), render.Style{Name: "rentopia-invoice", Stylesheet: stylesheet})
	})
	return htmlR, htmlErr
}

// HTML renders the invoice XML with go-ubltr's render module (helium
// XSLT 3, pure Go) and the module's stylesheet. The result has no scripts
// and loads nothing.
func HTML(ctx context.Context, xml []byte) ([]byte, error) {
	r, err := htmlRenderer()
	if err != nil {
		return nil, fmt.Errorf("ubl: html renderer: %w", err)
	}
	out, err := r.Render(ctx, xml)
	if err != nil {
		return nil, fmt.Errorf("ubl: render html: %w", err)
	}
	return out, nil
}
