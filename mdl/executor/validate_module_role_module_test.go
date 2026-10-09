// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// mendixlabs/mxcli#1354: `create module role NoSuchModule.Reader` passed
// `check -p`, and exec wrote the statements above it before stopping with
// "module not found: NoSuchModule". The module a module role lives in is
// resolved the way exec resolves it (findModule), or created above it.
func TestValidateModuleRole_ModuleMustExist(t *testing.T) {
	mod := mkModule("MyModule")
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleSecurityFunc: func(model.ID) (*security.ModuleSecurity, error) {
			return &security.ModuleSecurity{}, nil
		},
	}))

	cases := []struct {
		name, src string
		want      []string // substrings, one error each; nil = clean
	}{
		{"missing module", "create module role MyModule.Writer;\ncreate module role NoSuchModule.Reader;",
			[]string{"statement 2: module not found: NoSuchModule"}},
		{"create or modify", "create or modify module role NoSuchModule.Reader;",
			[]string{"statement 1: module not found: NoSuchModule"}},
		// Controls.
		{"stored module", "create module role MyModule.Writer;", nil},
		{"module the script creates", "create module NewMod;\ncreate module role NewMod.Reader;", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, err := range validateProgram(ctx, parseMDL(t, c.src)) {
				got = append(got, err.Error())
			}
			if len(got) != len(c.want) {
				t.Fatalf("want %d error(s) %q, got %q", len(c.want), c.want, got)
			}
			for i, w := range c.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("error %d: want %q in %q", i, w, got[i])
				}
			}
		})
	}
}
