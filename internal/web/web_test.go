package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go-data/internal/domain"
)

// Every element a page's scripts look up by id must exist in that page, so
// splitting markup into templates can't silently drop one.
func TestPagesHaveEveryIDTheirScriptsUse(t *testing.T) {
	pages, err := Render("../../web/templates", domain.HostInfo{Hostname: "test-host"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pages["/{$}"]), `id="hostname">test-host<`) {
		t.Error("hostname not rendered")
	}

	asset := regexp.MustCompile(`(?:src|href)="/((?:js|css)/[\w.-]+)"`)
	byID := regexp.MustCompile(`getElementById\("([\w-]+)"\)`)
	for route, page := range pages {
		html := string(page)
		assets := asset.FindAllStringSubmatch(html, -1)
		if len(assets) == 0 {
			t.Errorf("%s: no scripts or styles", route)
		}
		for _, a := range assets {
			src, err := os.ReadFile(filepath.Join("../../web/static", a[1]))
			if err != nil {
				t.Errorf("%s: %v", route, err)
				continue
			}
			for _, m := range byID.FindAllStringSubmatch(string(src), -1) {
				if !strings.Contains(html, `id="`+m[1]+`"`) {
					t.Errorf("%s: %s uses id %q, missing from page", route, a[1], m[1])
				}
			}
		}
	}
}
