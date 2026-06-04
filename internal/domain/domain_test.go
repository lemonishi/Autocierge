package domain

import "testing"

func TestDepartmentForType(t *testing.T) {
	cases := map[TicketType]Department{
		TypeBilling:        DeptBilling,
		TypeTechnical:      DeptEngineering,
		TypeAccount:        DeptAccounts,
		TypeFeatureRequest: DeptProduct,
		TypeGeneral:        DeptSupportTier1,
	}
	for tt, want := range cases {
		if got := DepartmentForType(tt); got != want {
			t.Fatalf("DepartmentForType(%q) = %q, want %q", tt, got, want)
		}
	}
}

func TestValidUrgency(t *testing.T) {
	if !ValidUrgency("critical") {
		t.Fatal("critical should be valid")
	}
	if ValidUrgency("meltdown") {
		t.Fatal("meltdown should be invalid")
	}
}
