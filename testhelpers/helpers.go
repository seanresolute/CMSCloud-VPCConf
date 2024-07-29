package testhelpers

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/testmocks"
	"github.com/davecgh/go-spew/spew"
)

func contains(target string, slice []string) bool {
	for _, item := range slice {
		if item == target {
			return true
		}
	}

	return false
}

func NewItems(startSlice, endSlice []string) []string {
	uniques := []string{}
	for _, item := range endSlice {
		if !contains(item, startSlice) {
			uniques = append(uniques, item)
		}
	}
	return uniques
}

func SortVPCState(state *database.VPCState) {
	for _, routeTable := range state.RouteTables {
		sort.Slice(routeTable.Routes, func(i, j int) bool {
			cidrI := strings.Split(routeTable.Routes[i].Destination, "/")
			cidrJ := strings.Split(routeTable.Routes[j].Destination, "/")

			ipI := net.ParseIP(cidrI[0])
			ipJ := net.ParseIP(cidrJ[0])
			if net.IP.Equal(ipI, ipJ) {
				sizeI, _ := strconv.Atoi(cidrI[1])
				sizeJ, _ := strconv.Atoi(cidrJ[1])
				return sizeI > sizeJ
			}
			return bytes.Compare(ipJ, ipI) > 0
		})
	}
}

func SortIpcontrolContainersAndBlocks(tree *testmocks.ContainerTree) {
	sort.Slice(tree.Children, func(i, j int) bool {
		return tree.Children[i].Name > tree.Children[j].Name
	})

	//Sort by size, if equal by address, if equal by status
	sort.Slice(tree.Blocks, func(i, j int) bool {
		if tree.Blocks[i].Size == tree.Blocks[j].Size {
			ipI := net.ParseIP(tree.Blocks[i].Address)
			ipJ := net.ParseIP(tree.Blocks[j].Address)
			if net.IP.Equal(ipI, ipJ) {
				return tree.Blocks[i].Status < tree.Blocks[j].Status
			}
			return bytes.Compare(ipJ, ipI) > 0
		}
		return tree.Blocks[i].Size > tree.Blocks[j].Size
	})

	for i := range tree.Children {
		SortIpcontrolContainersAndBlocks(&tree.Children[i])
	}
}

func ObjectGoPrintSideBySide(a, b interface{}) string {
	s := spew.ConfigState{
		Indent: " ",
		// Extra deep spew.
		DisableMethods: true,
		SortKeys:       true,
	}
	sA := s.Sdump(a)
	sB := s.Sdump(b)

	linesA := strings.Split(sA, "\n")
	linesB := strings.Split(sB, "\n")
	width := 0
	for _, s := range linesA {
		l := len(s)
		if l > width {
			width = l
		}
	}
	for _, s := range linesB {
		l := len(s)
		if l > width {
			width = l
		}
	}
	buf := &bytes.Buffer{}
	w := tabwriter.NewWriter(buf, width, 0, 1, ' ', 0)
	max := len(linesA)
	if len(linesB) > max {
		max = len(linesB)
	}
	for i := 0; i < max; i++ {
		var a, b string
		if i < len(linesA) {
			a = linesA[i]
		}
		if i < len(linesB) {
			b = linesB[i]
		}
		fmt.Fprintf(w, "%s\t%s\n", a, b)
	}
	w.Flush()
	return buf.String()
}
