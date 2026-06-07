package enginecompare

import "testing"

// dropOneAttr runs CREATE then ALTER…DROP ATTRIBUTE as two separate sessions so
// the drop loads the entity back from persisted disk — exercising the codec's
// passthrough dirty-path (kept attributes re-emitted from their original raw
// bytes, only the Attributes list rebuilt).
func dropAttrSeq(t *testing.T, eng Engine) string {
	t.Helper()
	p := copyProject(t)
	const create = "CREATE PERSISTENT ENTITY MyFirstModule.DropAttrTest " +
		"( Keep1: string(50), Gone: integer, Keep2: boolean )"
	const drop = "ALTER ENTITY MyFirstModule.DropAttrTest DROP ATTRIBUTE Gone"
	if _, e := Run(eng, p, create); e != nil {
		t.Fatalf("%s create: %v", eng, e)
	}
	if _, e := Run(eng, p, drop); e != nil {
		t.Fatalf("%s drop: %v", eng, e)
	}
	s, e := EntityCanonBSON(p, "MyFirstModule", "DropAttrTest")
	if e != nil {
		t.Fatalf("%s canon: %v", eng, e)
	}
	return s
}

func TestWriteParity_DropAttribute(t *testing.T) {
	leg := dropAttrSeq(t, Legacy)
	msd := dropAttrSeq(t, ModelSDK)
	if leg != msd {
		t.Errorf("DROP ATTRIBUTE divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd)
	}
}

// TestWriteParity_AlterKeepsAccessRule verifies that ALTERing an entity which
// already has an access rule preserves that rule (and its member accesses) on the
// codec round-trip. The role/entity/grant are set up via the legacy engine on
// both copies so the starting state is identical; only the final ALTER differs by
// engine. This is the path the UpdateEntity guard used to refuse.
func TestWriteParity_AlterKeepsAccessRule(t *testing.T) {
	const ent = "MyFirstModule.AREnt"
	setup := []string{
		"CREATE MODULE ROLE MyFirstModule.ARRole",
		"CREATE PERSISTENT ENTITY " + ent + " ( Code: string(20), Rank: integer )",
		"GRANT MyFirstModule.ARRole ON " + ent + " (read *, write *)",
	}
	alter := "ALTER ENTITY " + ent + " ADD ATTRIBUTE Extra: boolean"

	run := func(alterEng Engine) string {
		p := copyProject(t)
		for _, s := range setup {
			if _, e := Run(Legacy, p, s); e != nil {
				t.Fatalf("setup %q: %v", s, e)
			}
		}
		if _, e := Run(alterEng, p, alter); e != nil {
			t.Fatalf("%s alter: %v", alterEng, e)
		}
		s, e := EntityCanonBSON(p, "MyFirstModule", "AREnt")
		if e != nil {
			t.Fatalf("%s canon: %v", alterEng, e)
		}
		return s
	}
	if leg, msd := run(Legacy), run(ModelSDK); leg != msd {
		t.Errorf("ALTER-keeps-access-rule divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd)
	}
}

// alterSeq runs a CREATE then an ALTER as two sessions (so the ALTER loads the
// entity from disk: read-modify-write), returning the entity's canonical BSON.
func alterSeq(t *testing.T, eng Engine, create, alter, module, entity string) string {
	t.Helper()
	p := copyProject(t)
	if _, e := Run(eng, p, create); e != nil {
		t.Fatalf("%s create: %v", eng, e)
	}
	if _, e := Run(eng, p, alter); e != nil {
		t.Fatalf("%s alter %q: %v", eng, alter, e)
	}
	s, e := EntityCanonBSON(p, module, entity)
	if e != nil {
		t.Fatalf("%s canon: %v", eng, e)
	}
	return s
}

// TestWriteParity_AlterEntity exercises ALTER ENTITY read-modify-write across
// the codec engine vs legacy. Each case creates an entity then applies one ALTER
// op; both engines must produce identical canonical BSON. Cases touching indexes
// stress the lossless read adapter beyond attributes.
func TestWriteParity_AlterEntity(t *testing.T) {
	const ent = "MyFirstModule.AlterT"
	base := "CREATE PERSISTENT ENTITY " + ent + " ( Code: string(20), Rank: integer )"
	idxBase := base + " index (Code)"
	cases := []struct{ name, create, alter string }{
		{"AddAttribute", base, "ALTER ENTITY " + ent + " ADD ATTRIBUTE Extra: decimal"},
		{"RenameAttribute", base, "ALTER ENTITY " + ent + " RENAME ATTRIBUTE Rank TO Position"},
		{"ModifyAttribute", base, "ALTER ENTITY " + ent + " MODIFY ATTRIBUTE Code string(200)"},
		{"SetDocumentation", base, "ALTER ENTITY " + ent + " SET DOCUMENTATION 'an altered entity'"},
		{"AddIndex", base, "ALTER ENTITY " + ent + " ADD INDEX (Rank)"},
		{"DropIndexKeepsAttrs", idxBase, "ALTER ENTITY " + ent + " ADD ATTRIBUTE Extra: boolean"},
		{"AlterKeepsValidation",
			"CREATE PERSISTENT ENTITY " + ent + " ( Code: string(20) unique error 'must be unique', Rank: integer )",
			"ALTER ENTITY " + ent + " ADD ATTRIBUTE Extra: boolean"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			leg := alterSeq(t, Legacy, c.create, c.alter, "MyFirstModule", "AlterT")
			msd := alterSeq(t, ModelSDK, c.create, c.alter, "MyFirstModule", "AlterT")
			if leg != msd {
				t.Errorf("%s divergence:\nlegacy:   %s\nmodelsdk: %s", c.name, leg, msd)
			}
		})
	}
}

// dropEntitySeq creates two entities + an association between them, then drops
// the FROM entity. The kept entity's canonical BSON is returned; the cascade
// (entity + association removed) is asserted separately by the caller.
func dropEntitySeq(t *testing.T, eng Engine) string {
	t.Helper()
	p := copyProject(t)
	stmts := []string{
		"CREATE PERSISTENT ENTITY MyFirstModule.DropParent ( Code: string(20) )",
		"CREATE PERSISTENT ENTITY MyFirstModule.DropChild ( Label: string(20) )",
		"CREATE ASSOCIATION MyFirstModule.DropChild_DropParent FROM MyFirstModule.DropChild TO MyFirstModule.DropParent",
		"DROP ENTITY MyFirstModule.DropChild",
	}
	for _, s := range stmts {
		if _, e := Run(eng, p, s); e != nil {
			t.Fatalf("%s %q: %v", eng, s, e)
		}
	}
	// The dropped entity must be gone.
	if _, e := EntityCanonBSON(p, "MyFirstModule", "DropChild"); e == nil {
		t.Fatalf("%s: DropChild still present after DROP ENTITY", eng)
	}
	kept, e := EntityCanonBSON(p, "MyFirstModule", "DropParent")
	if e != nil {
		t.Fatalf("%s: kept entity canon: %v", eng, e)
	}
	return kept
}

func TestWriteParity_DropEntity(t *testing.T) {
	leg := dropEntitySeq(t, Legacy)
	msd := dropEntitySeq(t, ModelSDK)
	if leg != msd {
		t.Errorf("DROP ENTITY (kept entity) divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd)
	}
}
