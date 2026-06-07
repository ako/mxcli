package enginecompare

import "testing"

// TestWriteParity_Enumeration exercises enumeration writes (top-level documents,
// inserted as units) through the codec engine vs legacy: CREATE with plain and
// captioned values, and ALTER (read-modify-write) adding a value. Both engines
// must produce identical canonical BSON.
func TestWriteParity_Enumeration(t *testing.T) {
	t.Run("CreatePlain", func(t *testing.T) {
		const s = "CREATE ENUMERATION MyFirstModule.Status ( Open, Pending, Closed )"
		assertEnumParity(t, s, "", "Status")
	})
	t.Run("CreateCaptioned", func(t *testing.T) {
		const s = "CREATE ENUMERATION MyFirstModule.Prio ( Low 'Low prio', High 'High prio' )"
		assertEnumParity(t, s, "", "Prio")
	})
	t.Run("AlterAddValue", func(t *testing.T) {
		const create = "CREATE ENUMERATION MyFirstModule.Color ( Red, Green )"
		assertEnumParity(t, create, "ALTER ENUMERATION MyFirstModule.Color ADD VALUE Blue CAPTION 'Bluey'", "Color")
	})
}

// assertEnumParity runs create (and optional alter, as a separate session) on a
// copy per engine and compares the enumeration's canonical BSON.
func assertEnumParity(t *testing.T, create, alter, enumName string) {
	t.Helper()
	run := func(eng Engine) string {
		p := copyProject(t)
		if _, e := Run(eng, p, create); e != nil {
			t.Fatalf("%s create: %v", eng, e)
		}
		if alter != "" {
			if _, e := Run(eng, p, alter); e != nil {
				t.Fatalf("%s alter: %v", eng, e)
			}
		}
		s, e := EnumCanonBSON(p, "MyFirstModule", enumName)
		if e != nil {
			t.Fatalf("%s canon: %v", eng, e)
		}
		return s
	}
	if leg, msd := run(Legacy), run(ModelSDK); leg != msd {
		t.Errorf("enumeration divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd)
	}
}

// TestWriteParity_DropEnumeration verifies DROP ENUMERATION removes the unit in
// both engines.
func TestWriteParity_DropEnumeration(t *testing.T) {
	const create = "CREATE ENUMERATION MyFirstModule.Gone ( A, B )"
	const drop = "DROP ENUMERATION MyFirstModule.Gone"
	for _, eng := range []Engine{Legacy, ModelSDK} {
		p := copyProject(t)
		if _, e := Run(eng, p, create); e != nil {
			t.Fatalf("%s create: %v", eng, e)
		}
		if _, e := Run(eng, p, drop); e != nil {
			t.Fatalf("%s drop: %v", eng, e)
		}
		if _, e := EnumCanonBSON(p, "MyFirstModule", "Gone"); e == nil {
			t.Errorf("%s: enumeration still present after DROP", eng)
		}
	}
}
