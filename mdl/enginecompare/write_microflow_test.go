package enginecompare

import "testing"

// TestWriteParity_Microflow_ObjectOps validates the object-operations group
// (create/change/commit/delete/rollback) against legacy. The entity is set up via
// legacy on both copies; only the microflow CREATE differs by engine.
func TestWriteParity_Microflow_ObjectOps(t *testing.T) {
	const setup = "CREATE PERSISTENT ENTITY MyFirstModule.Thing ( Name: string(100), Count: integer )"
	cases := []struct{ name, stmt, mf string }{
		{"CreateChangeCommit",
			"CREATE MICROFLOW MyFirstModule.MfCCC () BEGIN\n" +
				"$New = create MyFirstModule.Thing (Name = 'x', Count = 5);\n" +
				"change $New (Count = 6);\n" +
				"commit $New;\n" +
				"END", "MfCCC"},
		{"CommitWithEvents",
			"CREATE MICROFLOW MyFirstModule.MfCE (Item: MyFirstModule.Thing) BEGIN commit $Item with events; END", "MfCE"},
		{"Delete",
			"CREATE MICROFLOW MyFirstModule.MfDel (Item: MyFirstModule.Thing) BEGIN delete $Item; END", "MfDel"},
		{"Rollback",
			"CREATE MICROFLOW MyFirstModule.MfRb (Item: MyFirstModule.Thing) BEGIN rollback $Item; END", "MfRb"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := func(eng Engine) string {
				p := copyProject(t)
				if _, e := Run(Legacy, p, setup); e != nil {
					t.Fatalf("setup: %v", e)
				}
				if _, e := Run(eng, p, c.stmt); e != nil {
					t.Fatalf("%s create: %v", eng, e)
				}
				s, e := MicroflowCanonBSON(p, "MyFirstModule", c.mf)
				if e != nil {
					t.Fatalf("%s canon: %v", eng, e)
				}
				return s
			}
			if leg, msd := run(Legacy), run(ModelSDK); leg != msd {
				t.Errorf("%s divergence:\nlegacy:   %s\nmodelsdk: %s", c.name, leg, msd)
			}
		})
	}
}

// TestWriteParity_Microflow_Calls validates the call group: microflow calls
// (with/without args + result) and nanoflow calls, each with a nested Call
// element + parameter mappings (marker-2 list).
func TestWriteParity_Microflow_Calls(t *testing.T) {
	setup := []string{
		"CREATE MICROFLOW MyFirstModule.CTarget () RETURNS BOOLEAN BEGIN RETURN true END",
		"CREATE MICROFLOW MyFirstModule.CTargetP (Val: string) RETURNS STRING BEGIN RETURN $Val END",
		"CREATE NANOFLOW MyFirstModule.NTarget () RETURNS BOOLEAN BEGIN RETURN true END",
	}
	cases := []struct{ name, stmt, mf string }{
		{"MicroflowNoArgs", "CREATE MICROFLOW MyFirstModule.MfCall () BEGIN call microflow MyFirstModule.CTarget(); END", "MfCall"},
		{"MicroflowArgResult", "CREATE MICROFLOW MyFirstModule.MfCallR () BEGIN $R = call microflow MyFirstModule.CTargetP(Val = 'x'); END", "MfCallR"},
		{"NanoflowCall", "CREATE MICROFLOW MyFirstModule.MfNano () BEGIN call nanoflow MyFirstModule.NTarget(); END", "MfNano"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := func(eng Engine) string {
				p := copyProject(t)
				for _, s := range setup {
					if _, e := Run(Legacy, p, s); e != nil {
						t.Fatalf("setup %q: %v", s, e)
					}
				}
				if _, e := Run(eng, p, c.stmt); e != nil {
					t.Fatalf("%s create: %v", eng, e)
				}
				s, e := MicroflowCanonBSON(p, "MyFirstModule", c.mf)
				if e != nil {
					t.Fatalf("%s canon: %v", eng, e)
				}
				return s
			}
			if leg, msd := run(Legacy), run(ModelSDK); leg != msd {
				t.Errorf("%s divergence:\nlegacy:   %s\nmodelsdk: %s", c.name, leg, msd)
			}
		})
	}
}

// TestWriteParity_Microflow_Loops validates the loop group: iterate-over-list
// (IterableList) and while (WhileLoopCondition), each with a nested body
// (ObjectCollection objects-only) and supported branch activities.
func TestWriteParity_Microflow_Loops(t *testing.T) {
	const setup = "CREATE PERSISTENT ENTITY MyFirstModule.LThing ( Code: string(20) )"
	cases := []struct{ name, stmt, mf string }{
		{"IterateList",
			"CREATE MICROFLOW MyFirstModule.MfLoop (Items: list of MyFirstModule.LThing) BEGIN " +
				"loop $It in $Items begin commit $It; end loop END", "MfLoop"},
		{"While",
			"CREATE MICROFLOW MyFirstModule.MfWhile (Item: MyFirstModule.LThing) BEGIN " +
				"while $Item/Code != '' begin commit $Item; end while END", "MfWhile"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := func(eng Engine) string {
				p := copyProject(t)
				if _, e := Run(Legacy, p, setup); e != nil {
					t.Fatalf("setup: %v", e)
				}
				if _, e := Run(eng, p, c.stmt); e != nil {
					t.Fatalf("%s create: %v", eng, e)
				}
				s, e := MicroflowCanonBSON(p, "MyFirstModule", c.mf)
				if e != nil {
					t.Fatalf("%s canon: %v", eng, e)
				}
				return s
			}
			if leg, msd := run(Legacy), run(ModelSDK); leg != msd {
				t.Errorf("%s divergence:\nlegacy:   %s\nmodelsdk: %s", c.name, leg, msd)
			}
		})
	}
}

// TestWriteParity_Microflow_Splits validates the exclusive-split group: an if/else
// (ExclusiveSplit + ExpressionSplitCondition + ExclusiveMerge + branch flows whose
// cases serialize as EnumerationCase true/false), with supported branch activities.
func TestWriteParity_Microflow_Splits(t *testing.T) {
	const setup = "CREATE PERSISTENT ENTITY MyFirstModule.SThing ( Count: integer )"
	const mf = "CREATE MICROFLOW MyFirstModule.MfSplit (Item: MyFirstModule.SThing) BEGIN " +
		"if $Item/Count > 10 then commit $Item; else rollback $Item; end if; END"
	run := func(eng Engine) string {
		p := copyProject(t)
		if _, e := Run(Legacy, p, setup); e != nil {
			t.Fatalf("setup: %v", e)
		}
		if _, e := Run(eng, p, mf); e != nil {
			t.Fatalf("%s create: %v", eng, e)
		}
		s, e := MicroflowCanonBSON(p, "MyFirstModule", "MfSplit")
		if e != nil {
			t.Fatalf("%s canon: %v", eng, e)
		}
		return s
	}
	if leg, msd := run(Legacy), run(ModelSDK); leg != msd {
		t.Errorf("split divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd)
	}
}

// TestWriteParity_Microflow_Retrieve validates the retrieve group: database
// source (all + XPath) and association source. Real BSON (test7) confirmed a
// DatabaseRetrieveSource always carries a default Range + an empty NewSortings.
func TestWriteParity_Microflow_Retrieve(t *testing.T) {
	setup := []string{
		"CREATE PERSISTENT ENTITY MyFirstModule.RThing ( Code: string(20), Count: integer )",
		"CREATE PERSISTENT ENTITY MyFirstModule.ROther ( Label: string(20) )",
		"CREATE ASSOCIATION MyFirstModule.RThing_ROther FROM MyFirstModule.RThing TO MyFirstModule.ROther",
	}
	cases := []struct{ name, stmt, mf string }{
		{"DatabaseAll", "CREATE MICROFLOW MyFirstModule.MfRetAll () BEGIN retrieve $L from MyFirstModule.RThing; END", "MfRetAll"},
		{"DatabaseWhere", "CREATE MICROFLOW MyFirstModule.MfRetWhere () BEGIN retrieve $L from MyFirstModule.RThing where Count = 5; END", "MfRetWhere"},
		{"Association", "CREATE MICROFLOW MyFirstModule.MfRetAssoc (Item: MyFirstModule.RThing) BEGIN retrieve $L from $Item/MyFirstModule.RThing_ROther; END", "MfRetAssoc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := func(eng Engine) string {
				p := copyProject(t)
				for _, s := range setup {
					if _, e := Run(Legacy, p, s); e != nil {
						t.Fatalf("setup %q: %v", s, e)
					}
				}
				if _, e := Run(eng, p, c.stmt); e != nil {
					t.Fatalf("%s create: %v", eng, e)
				}
				s, e := MicroflowCanonBSON(p, "MyFirstModule", c.mf)
				if e != nil {
					t.Fatalf("%s canon: %v", eng, e)
				}
				return s
			}
			if leg, msd := run(Legacy), run(ModelSDK); leg != msd {
				t.Errorf("%s divergence:\nlegacy:   %s\nmodelsdk: %s", c.name, leg, msd)
			}
		})
	}
}

// TestWriteParity_Microflow validates the codec-native microflow CREATE path
// against legacy, group by group. Skeleton = start → end, boolean return.
func TestWriteParity_Microflow(t *testing.T) {
	cases := []struct{ name, stmt, mf string }{
		{"Skeleton", "CREATE MICROFLOW MyFirstModule.MfEmpty () RETURNS BOOLEAN BEGIN RETURN true END", "MfEmpty"},
		{"Parameters", "CREATE MICROFLOW MyFirstModule.MfParams (Count: integer, Label: string) RETURNS BOOLEAN BEGIN RETURN true END", "MfParams"},
		{"VoidReturn", "CREATE MICROFLOW MyFirstModule.MfVoid () BEGIN END", "MfVoid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			run := func(eng Engine) string {
				p := copyProject(t)
				if _, e := Run(eng, p, c.stmt); e != nil {
					t.Fatalf("%s create: %v", eng, e)
				}
				s, e := MicroflowCanonBSON(p, "MyFirstModule", c.mf)
				if e != nil {
					t.Fatalf("%s canon: %v", eng, e)
				}
				return s
			}
			if leg, msd := run(Legacy), run(ModelSDK); leg != msd {
				t.Errorf("%s divergence:\nlegacy:   %s\nmodelsdk: %s", c.name, leg, msd)
			}
		})
	}
}
