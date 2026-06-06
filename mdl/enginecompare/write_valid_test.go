package enginecompare
import "testing"
func TestWriteParity_ValidationRules(t *testing.T) {
	const s = "CREATE PERSISTENT ENTITY MyFirstModule.ValTest " +
		"( Name: string(100) not null error 'Name is required', Code: string(20) unique error 'Code must be unique' )"
	lp := copyProject(t); if _,e:=Run(Legacy,lp,s);e!=nil{t.Fatalf("legacy: %v",e)}
	mp := copyProject(t); if _,e:=Run(ModelSDK,mp,s);e!=nil{t.Fatalf("modelsdk: %v",e)}
	leg,e:=EntityCanonBSON(lp,"MyFirstModule","ValTest"); if e!=nil{t.Fatalf("leg: %v",e)}
	msd,e:=EntityCanonBSON(mp,"MyFirstModule","ValTest"); if e!=nil{t.Fatalf("msd: %v",e)}
	if leg!=msd { t.Errorf("ValidationRules divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd) }
}
