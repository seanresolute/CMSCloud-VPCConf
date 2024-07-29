package database

import (
	"go/constant"
	"go/types"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/tools/go/packages"
)

func TestAllSubnetTypes(t *testing.T) {
	// There's no built-in way to iterate through const declarations, so we have
	// to parse the code and find the declarations there.
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedTypesInfo}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatalf("Error loading: %s", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("There were errors")
	}

	expectedAllSubnetTypes := []SubnetType{}
	alreadyFound := func(subnetType SubnetType) bool {
		for _, st := range expectedAllSubnetTypes {
			if st == subnetType {
				return true
			}
		}
		return false
	}

	for _, pkg := range pkgs {
		// Collect all identifiers with constant value and type=SubnetType
		for ident, obj := range pkg.TypesInfo.Defs {
			if obj != nil {
				if ident.Obj != nil && ident.Obj.Decl != nil {
					if c, ok := obj.(*types.Const); ok {
						if strings.HasSuffix(c.Type().String(), "database.SubnetType") {
							val := SubnetType(constant.StringVal(c.Val()))
							// This loop seems to include identifiers that have constant value but are not
							// from constant declarations, so filter out duplicates.
							if !alreadyFound(val) {
								expectedAllSubnetTypes = append(expectedAllSubnetTypes, val)
							}
						}
					}
				}
			}
		}
	}

	allSubnetTypes := AllSubnetTypes()
	byValue := func(s []SubnetType) func(i, j int) bool {
		return func(i, j int) bool {
			return s[i] < s[j]
		}
	}
	sort.Slice(allSubnetTypes, byValue(allSubnetTypes))
	sort.Slice(expectedAllSubnetTypes, byValue(expectedAllSubnetTypes))
	if diff := cmp.Diff(expectedAllSubnetTypes, allSubnetTypes); diff != "" {
		t.Fatalf("Not all subnet types returned by AllSubnetTypes(): \n%s", diff)
	}
}
