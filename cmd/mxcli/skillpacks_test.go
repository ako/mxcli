// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/skillpack"
)

// TestVendoredPacksLoad checks every pack that actually ships in this binary.
// The unit tests in cmd/mxcli/skillpack prove the mechanism against fixtures;
// this proves the vendored content matches it. A pack that is broken only when
// vendored fails at somebody's install otherwise, which is the worst place to
// find out.
func TestVendoredPacksLoad(t *testing.T) {
	fsys, err := packsFS()
	if err != nil {
		t.Fatalf("packsFS: %v", err)
	}
	packs, err := skillpack.List(fsys)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(packs) == 0 {
		t.Fatal("no packs are embedded; run `make sync-skill-packs`")
	}

	for _, p := range packs {
		t.Run(p.Name, func(t *testing.T) {
			if p.Version == "" {
				t.Error("no version")
			}
			if p.Description == "" {
				t.Error("no description")
			}
			// Every file the manifest says it rewrites must exist and carry a
			// token. Install enforces this too, but only when someone installs.
			for _, rel := range p.Rewrite.Files {
				full := p.Dir + "/" + rel
				body, err := fs.ReadFile(fsys, full)
				if err != nil {
					t.Errorf("rewrite.files names %s, which is not shipped: %v", rel, err)
					continue
				}
				if !regexp.MustCompile(`\{\{[A-Z_]+\}\}`).Match(body) {
					t.Errorf("%s is listed under rewrite.files but carries no {{TOKEN}}", rel)
				}
			}
			// Anything the manifest promises to install has to be there.
			for _, w := range p.Installs.Widgets {
				if _, err := fs.Stat(fsys, p.Dir+"/"+w); err != nil {
					t.Errorf("installs.widgets names %s, which is not shipped: %v", w, err)
				}
			}
			for _, m := range p.Installs.MDL {
				if _, err := fs.Stat(fsys, p.Dir+"/"+m); err != nil {
					t.Errorf("installs.mdl names %s, which is not shipped: %v", m, err)
				}
			}
			for _, j := range p.Installs.Java {
				if _, err := fs.Stat(fsys, p.Dir+"/"+j); err != nil {
					t.Errorf("installs.java names %s, which is not shipped: %v", j, err)
				}
			}
		})
	}
}

// TestVendoredPacksCarryNoForeignNamespace is the vendoring hazard with teeth.
//
// These packs came from a real project, and a widget id is identity: if one
// ships with `ledger.widget.web` still in it, every project installing it builds
// a widget claiming to be the ledger's. That is not a build error — it is two
// apps whose widgets collide, discovered late.
//
// The check is deliberately on the widget source only. Prose may name the
// project it came from, and the specs are the ledger's own data.
func TestVendoredPacksCarryNoForeignNamespace(t *testing.T) {
	fsys, err := packsFS()
	if err != nil {
		t.Fatalf("packsFS: %v", err)
	}
	// Namespaces of the projects these packs were harvested from. A pack must
	// carry a placeholder instead.
	foreign := []string{"ledger.widget", "ledger/widget"}

	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.Contains(p, "/widget/") {
			return nil
		}
		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		for _, f := range foreign {
			if strings.Contains(string(body), f) {
				t.Errorf("%s still carries the harvested project's namespace (%q); "+
					"it must be a {{NAMESPACE}} placeholder", p, f)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWidgetPacksShipALockfile — a pack telling the reader to run `npm ci` must
// ship the lockfile that command requires.
//
// This is not hypothetical tidiness: mendix-vega-charts shipped documenting
// `npm ci` with no lock, so the one command the pack told people to run failed
// on the spot ("can only install with an existing package-lock.json"). It
// survived an end-to-end verification because that run used `npm install` —
// proving the build worked while never exercising the documented path.
//
// The check is anchored on package.json rather than on the prose: a pack that
// builds JavaScript wants a reproducible tree whatever its docs happen to say.
// Direct dependencies being exact-pinned is not enough, because the transitive
// tree is not, and that drift surfaces as a compile error in somebody else's
// project long after anyone chose an upgrade.
func TestWidgetPacksShipALockfile(t *testing.T) {
	fsys, err := packsFS()
	if err != nil {
		t.Fatalf("packsFS: %v", err)
	}
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "package.json" {
			return err
		}
		// Only the widget's own manifest; a scripts/ helper is not a build.
		if !strings.Contains(p, "/widget/") {
			return nil
		}
		lock := strings.TrimSuffix(p, "package.json") + "package-lock.json"
		if _, err := fs.Stat(fsys, lock); err != nil {
			t.Errorf("%s ships no %s; `npm ci` cannot run and the build is not reproducible",
				p, filepath.Base(lock))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestUrlFedPacksDocumentSessionAuth — a pack that tells the reader to fetch
// data from a relative URL must say how that request authenticates.
//
// ako/mxcli#574: mendix-vega-charts documented a `{"data": {"url": "/odata/…"}}`
// spec and stated that "same-origin requests carry the session cookie, so an
// endpoint authenticated by session is reachable … without any token handling".
// The cookie does go. It is not sufficient — Mendix answers a
// session-authenticated OData READ 401 without `X-Csrf-Token` (measured on
// 11.14.0: cookie only 401, cookie + header 200), and Vega reads the 401 body as
// an empty dataset, so the chart draws axes and a full legend with no marks and
// no error.
//
// The check is on the *vendored* copy, which is what installs, and it is keyed
// on the relative URL in the docs rather than on this one pack: any pack that
// documents fetching from the app's own endpoints has the same obligation.
func TestUrlFedPacksDocumentSessionAuth(t *testing.T) {
	fsys, err := packsFS()
	if err != nil {
		t.Fatalf("packsFS: %v", err)
	}
	// A data spec whose url is a relative path — i.e. the app's own endpoint.
	relativeURLSpec := regexp.MustCompile(`"url"\s*:\s*"/`)

	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		if !relativeURLSpec.Match(body) {
			return nil
		}
		if !strings.Contains(string(body), "X-Csrf-Token") {
			t.Errorf("%s documents fetching from a relative URL but never mentions "+
				"X-Csrf-Token; a session-authenticated read is refused 401 without it, "+
				"and the chart renders empty with no error (ako/mxcli#574)", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestCsrfTokenIsNeverSentCrossOrigin — the other half of #574's fix.
//
// The CSRF token authenticates this session against this app, so a widget that
// attaches it to whatever URL a spec names would hand a third-party host a
// working credential. A widget source that sets the header must therefore
// decide same-origin by RESOLVING the uri against the page, not by testing the
// string for a scheme: a protocol-relative "//elsewhere.example/rows.json"
// passes any `^[a-z][a-z0-9+.-]*://` test and goes to another host.
//
// The decision itself is unit-tested in the pack
// (widget/test/csrf.test.ts, `make check-skill-pack-js`). This guards the
// vendored copy against losing it.
func TestCsrfTokenIsNeverSentCrossOrigin(t *testing.T) {
	fsys, err := packsFS()
	if err != nil {
		t.Fatalf("packsFS: %v", err)
	}
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		// The widget's own tests name the header while asserting it is absent,
		// which is the opposite of the hazard.
		if err != nil || d.IsDir() || !strings.Contains(p, "/widget/") || strings.Contains(p, ".test.") {
			return err
		}
		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		src := string(body)
		if !strings.Contains(src, "X-Csrf-Token") {
			return nil
		}
		if !strings.Contains(src, "new URL(") {
			t.Errorf("%s attaches X-Csrf-Token but does not resolve the uri with new URL(); "+
				"a scheme test on the raw string sends this app's session token to "+
				"//another.host/x (ako/mxcli#574)", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestLockfilesAreNotRewritten — a lockfile must never be listed under
// rewrite.files. Substitution is what keeps a widget id unique, but a lock
// records resolved integrity hashes: rewriting one silently invalidates them
// and `npm ci` fails on a checksum, which reads as a corrupt registry rather
// than as a packaging mistake.
func TestLockfilesAreNotRewritten(t *testing.T) {
	fsys, err := packsFS()
	if err != nil {
		t.Fatalf("packsFS: %v", err)
	}
	packs, err := skillpack.List(fsys)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, p := range packs {
		for _, f := range p.Rewrite.Files {
			if strings.HasSuffix(f, "package-lock.json") {
				t.Errorf("%s: %s is listed under rewrite.files; a lockfile must be shipped verbatim", p.Name, f)
			}
		}
	}
}
