package enginecompare

import "testing"

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
