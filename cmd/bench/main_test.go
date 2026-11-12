package main

import "testing"

func TestPercentileNearestRank(t *testing.T) {
	values:=[]int64{1000,2000,3000,4000}
	tests:=[]struct{name string;p,want float64}{
		{name:"p50",p:.50,want:2},
		{name:"p95",p:.95,want:4},
		{name:"p999",p:.999,want:4},
	}
	for _,tt:=range tests { t.Run(tt.name,func(t *testing.T){ if got:=percentile(values,tt.p);got!=tt.want { t.Fatalf("got %v want %v",got,tt.want) } }) }
}
func TestPercentileEmpty(t *testing.T) {
	if got:=percentile(nil,.99);got!=0 { t.Fatalf("got %v want 0",got) }
}
func TestMakeValue(t *testing.T) {
	if got:=makeValue(3);got!="xxx" { t.Fatalf("got %q want xxx",got) }
	if got:=makeValue(0);got!="" { t.Fatalf("got %q want empty",got) }
}
func TestMakeKeys(t *testing.T) {
	got:=makeKeys(2,5)
	if len(got)!=3 || got[0]!="bench:key:2" || got[2]!="bench:key:4" { t.Fatalf("unexpected keys: %#v",got) }
}
