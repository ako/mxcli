package enginecompare

import "testing"

// TestWriteParity_MoveEntity moves an entity (whose association to a same-module
// entity becomes a cross-module association) and checks the moved entity matches
// legacy in the target module. Cross-association correctness is covered by mx check.
func TestWriteParity_MoveEntity(t *testing.T) {
	setup := []string{
		"CREATE MODULE SrcM",
		"CREATE MODULE DstM",
		"CREATE PERSISTENT ENTITY SrcM.Parent ( Code: string(20) )",
		"CREATE PERSISTENT ENTITY SrcM.Child ( Label: string(20) )",
		"CREATE ASSOCIATION SrcM.Parent_Child FROM SrcM.Parent TO SrcM.Child TYPE reference OWNER default",
		"MOVE ENTITY SrcM.Child TO DstM",
	}
	run := func(eng Engine) string {
		p := copyProject(t)
		for _, s := range setup {
			if _, e := Run(eng, p, s); e != nil {
				t.Fatalf("%s %q: %v", eng, s, e)
			}
		}
		s, e := EntityCanonBSON(p, "DstM", "Child")
		if e != nil {
			t.Fatalf("%s canon: %v", eng, e)
		}
		return s
	}
	if leg, msd := run(Legacy), run(ModelSDK); leg != msd {
		t.Errorf("moved-entity divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd)
	}
}

// TestWriteParity_MoveViewEntity moves a view entity to another module. This
// exercises the reference-cleanup helpers that keep the moved view consistent:
// MoveViewEntitySourceDocument (reparent the OQL source doc, else CE6786) and the
// OqlViewValue round-trip in attributeFromGen (else the rebuilt view goes out of
// sync, CE6770). The moved entity's BSON must match legacy in the target module.
func TestWriteParity_MoveViewEntity(t *testing.T) {
	setup := []string{
		"CREATE MODULE SrcV",
		"CREATE MODULE DstV",
		"CREATE PERSISTENT ENTITY SrcV.Base ( Name: string(50), Amount: integer )",
		"CREATE OR MODIFY VIEW ENTITY SrcV.BaseView ( Name: string(50) ) AS select b.Name as Name from SrcV.Base as b",
		"MOVE ENTITY SrcV.BaseView TO DstV",
	}
	run := func(eng Engine) string {
		p := copyProject(t)
		for _, s := range setup {
			if _, e := Run(eng, p, s); e != nil {
				t.Fatalf("%s %q: %v", eng, s, e)
			}
		}
		s, e := EntityCanonBSON(p, "DstV", "BaseView")
		if e != nil {
			t.Fatalf("%s canon: %v", eng, e)
		}
		return s
	}
	if leg, msd := run(Legacy), run(ModelSDK); leg != msd {
		t.Errorf("moved-view-entity divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd)
	}
}
