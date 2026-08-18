// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	mdlast "github.com/mendixlabs/mxcli/mdl/ast"

	"os"
	"regexp"
	"strings"
	"testing"
)

// TestEveryShowAlternativeProducesAStatement is the drift guard for a whole
// class of silent failure: a grammar alternative that parses cleanly while the
// visitor has no branch for it, so it produces NO AST statement at all.
//
// The result is the worst possible shape for a CLI — `mxcli -c "show page X"`
// exits 0, prints nothing, and writes nothing. It is indistinguishable from
// "the query ran and there was nothing to show", so it reads as an empty
// project rather than as an unimplemented command. A parse error would have
// been strictly better. Four commands were in this state when the guard was
// written (`show page`, `show connections`, `show notebooks`, and
// `create notebook`).
//
// A structural check on the visitor source cannot catch this: `ctx.PAGE()` DOES
// appear in ExitShowStatement — for the unrelated `SHOW ACCESS ON PAGE`
// alternative — so grepping for the token says "covered" while the singular
// PAGE alternative is unhandled. Only running the text through the real parser
// and visitor distinguishes them.
func TestEveryShowAlternativeProducesAStatement(t *testing.T) {
	probes, skipped := showProbesFromGrammar(t)
	if len(probes) < 60 {
		t.Fatalf("only synthesized %d probes; the grammar reader is broken, "+
			"so a pass here would prove nothing", len(probes))
	}

	for _, p := range probes {
		prog, errs := Build(p)
		if len(errs) > 0 {
			// A probe the synthesizer got wrong is not a product bug. It must
			// stay rare, or this guard quietly stops covering the grammar.
			skipped = append(skipped, p)
			continue
		}
		if prog == nil || len(prog.Statements) == 0 {
			t.Errorf("`%s` parses but produces no AST statement — it will exit 0 "+
				"and print nothing, which reads as an empty result rather than an "+
				"unimplemented command. Add a visitor branch, or remove the "+
				"grammar alternative so it fails loudly.", p)
		}
	}

	if len(skipped) > len(probes)/4 {
		t.Errorf("%d of %d probes were skipped; the guard is no longer covering "+
			"most of the grammar:\n  %s", len(skipped), len(probes)+len(skipped),
			strings.Join(skipped, "\n  "))
	}
}

var (
	reGrammarComment = regexp.MustCompile(`//.*$`)
	reOptionalToken  = regexp.MustCompile(`\b([A-Z_]+)\?`)
	reAllCaps        = regexp.MustCompile(`^[A-Z_]+$`)
)

// showProbesFromGrammar turns each `showOrList …` alternative of the
// showStatement rule into a concrete command string. Optional groups are
// dropped (the shortest legal form is the one worth probing) and the few
// alternatives whose tail is a sub-rule are reported as skipped rather than
// guessed at.
func showProbesFromGrammar(t *testing.T) (probes, skipped []string) {
	t.Helper()
	src, err := os.ReadFile("../grammar/domains/MDLCatalog.g4")
	if err != nil {
		t.Fatalf("read grammar: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "showStatement")
	if start < 0 {
		t.Fatal("no showStatement rule in MDLCatalog.g4")
	}
	body = body[start:]
	if end := strings.Index(body, "\n    ;"); end > 0 {
		body = body[:end]
	}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		alt := strings.TrimSpace(reGrammarComment.ReplaceAllString(strings.TrimLeft(line, "| "), ""))
		if !strings.HasPrefix(alt, "showOrList") {
			continue
		}
		alt = strings.TrimSpace(strings.TrimPrefix(alt, "showOrList"))
		alt = dropOptionalGroups(alt)
		alt = reOptionalToken.ReplaceAllString(alt, "")

		words := []string{"show"}
		unsynthesizable := false
		for _, tok := range strings.Fields(alt) {
			switch {
			case tok == "qualifiedName":
				words = append(words, "Mod.Name")
			case tok == "NUMBER_LITERAL":
				words = append(words, "11")
			case tok == "MENU_KW":
				words = append(words, "menu")
			case reAllCaps.MatchString(tok):
				words = append(words, strings.ToLower(strings.TrimSuffix(tok, "_KW")))
			default:
				unsynthesizable = true // a sub-rule tail; don't guess
			}
			if unsynthesizable {
				break
			}
		}
		if unsynthesizable {
			skipped = append(skipped, alt)
			continue
		}
		probes = append(probes, strings.Join(words, " "))
	}
	return probes, skipped
}

// dropOptionalGroups removes every balanced `( … )?` group. Nesting is real in
// this grammar — `(IN (qualifiedName | IDENTIFIER))?` — so a regex is not
// enough.
func dropOptionalGroups(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '(' {
			out.WriteByte(s[i])
			i++
			continue
		}
		depth, j := 0, i
		for ; j < len(s); j++ {
			if s[j] == '(' {
				depth++
			} else if s[j] == ')' {
				if depth--; depth == 0 {
					break
				}
			}
		}
		if j+1 < len(s) && s[j+1] == '?' {
			i = j + 2 // drop the optional group entirely
			continue
		}
		out.WriteString(s[i : j+1])
		i = j + 1
	}
	return out.String()
}

// TestShowPageIsDescribePageAlias pins the shape, not just the presence, of the
// singular SHOW PAGE branch: it must produce a DescribeStmt, so `show page X`
// and `describe page X` agree — including reporting a missing page instead of
// exiting 0 in silence.
func TestShowPageIsDescribePageAlias(t *testing.T) {
	prog, errs := Build("show page Mod.Home")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(prog.Statements))
	}
	d, ok := prog.Statements[0].(*mdlast.DescribeStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.DescribeStmt — SHOW PAGE must reuse the "+
			"describe path so both spellings behave identically", prog.Statements[0])
	}
	if d.ObjectType != mdlast.DescribePage {
		t.Errorf("ObjectType = %v, want DescribePage", d.ObjectType)
	}
	if d.Name.String() != "Mod.Home" {
		t.Errorf("Name = %q, want Mod.Home", d.Name.String())
	}
}

// TestShowConnectionsProducesShowStmt covers the other silent no-op. SHOW
// CONNECTIONS lists live SQL sessions; SHOW DATABASE CONNECTIONS lists stored
// DatabaseConnector documents. They must not collapse into one another.
func TestShowConnectionsProducesShowStmt(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want mdlast.ShowObjectType
	}{
		{"show connections", mdlast.ShowConnections},
		{"show database connections", mdlast.ShowDatabaseConnections},
	} {
		prog, errs := Build(tc.src)
		if len(errs) > 0 {
			t.Fatalf("parse %q: %v", tc.src, errs)
		}
		if len(prog.Statements) != 1 {
			t.Fatalf("%q: got %d statements, want 1", tc.src, len(prog.Statements))
		}
		s, ok := prog.Statements[0].(*mdlast.ShowStmt)
		if !ok {
			t.Fatalf("%q: got %T, want *ast.ShowStmt", tc.src, prog.Statements[0])
		}
		if s.ObjectType != tc.want {
			t.Errorf("%q: ObjectType = %v, want %v", tc.src, s.ObjectType, tc.want)
		}
	}
}
