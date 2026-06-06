package enginecompare
import "testing"
func TestWriteParity_Generalization(t *testing.T) {
	const s = "CREATE PERSISTENT ENTITY MyFirstModule.GenParent;CREATE PERSISTENT ENTITY MyFirstModule.GenChild EXTENDS MyFirstModule.GenParent;"
	lp := copyProject(t); if _,err:=Run(Legacy,lp,s);err!=nil{t.Fatalf("legacy: %v",err)}
	mp := copyProject(t); if _,err:=Run(ModelSDK,mp,s);err!=nil{t.Fatalf("modelsdk: %v",err)}
	leg,err:=EntityCanonBSON(lp,"MyFirstModule","GenChild"); if err!=nil{t.Fatalf("leg: %v",err)}
	msd,err:=EntityCanonBSON(mp,"MyFirstModule","GenChild"); if err!=nil{t.Fatalf("msd: %v",err)}
	if leg!=msd { t.Errorf("Generalization divergence:\nlegacy:   %s\nmodelsdk: %s", leg, msd) }
}
