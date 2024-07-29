package client

import (
	"fmt"
	"sort"
	"testing"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/swagger/models"
)

// From https://stackoverflow.com/a/30230552
func nextPerm(p []int) {
	for i := len(p) - 1; i >= 0; i-- {
		if i == 0 || p[i] < len(p)-i-1 {
			p[i]++
			return
		}
		p[i] = 0
	}
}
func getPerm(blocks Blocks, p []int) Blocks {
	result := Blocks(append([]*models.WSChildBlock{}, blocks...))
	for i, v := range p {
		result[i], result[i+v] = result[i+v], result[i]
	}
	return result
}

func checkBlockOrder(t *testing.T, expected, actual Blocks) {
	if len(actual) != len(expected) {
		t.Fatalf("Sorting failed: expected %d blocks but got %d", len(expected), len(actual))
	}
	for idx, actualBlock := range actual {
		expectedBlock := expected[idx]
		if actualBlock.BlockAddr != expectedBlock.BlockAddr || actualBlock.BlockSize != expectedBlock.BlockSize {
			t.Fatalf("Sorting failed: element %d: expected %s but got %s",
				idx,
				fmt.Sprintf("%s/%s", expectedBlock.BlockAddr, expectedBlock.BlockSize),
				fmt.Sprintf("%s/%s", actualBlock.BlockAddr, actualBlock.BlockSize))
			return
		}
	}
}

func TestSortBlocks(t *testing.T) {
	sortedBlocks := Blocks{
		&models.WSChildBlock{
			BlockAddr: "10.231.65.128",
			BlockSize: "25",
		},
		&models.WSChildBlock{
			BlockAddr: "10.231.65.160",
			BlockSize: "27",
		},
		&models.WSChildBlock{
			BlockAddr: "10.231.65.192",
			BlockSize: "27",
		},
		&models.WSChildBlock{
			BlockAddr: "10.231.66.0",
			BlockSize: "24",
		},
		&models.WSChildBlock{
			BlockAddr: "10.231.66.0",
			BlockSize: "26",
		},
		&models.WSChildBlock{
			BlockAddr: "10.231.66.64",
			BlockSize: "26",
		},
		&models.WSChildBlock{
			BlockAddr: "10.231.66.128",
			BlockSize: "26",
		},
		&models.WSChildBlock{
			BlockAddr: "10.231.66.128",
			BlockSize: "27",
		},
	}
	for p := make([]int, len(sortedBlocks)); p[0] < len(p); nextPerm(p) {
		perm := getPerm(sortedBlocks, p)
		sort.Sort(perm)
		checkBlockOrder(t, sortedBlocks, perm)
	}
}

func TestChooseBlock(t *testing.T) {
	type testCase struct {
		Size        int
		FreeSizes   []int
		ChoiceIndex int
	}
	testCases := []*testCase{
		{24, []int{24, 26, 24}, 0},
		{24, []int{23, 26, 24}, 2},
		{25, []int{26, 24, 24}, 1},
		{23, []int{24, 25, 26}, -1},
		{23, []int{24, 24, 24}, -1},
		{23, []int{27, 25, 26, 22, 24, 23, 28}, 5},
		{26, []int{26}, 0},
		{26, []int{}, -1},
		{26, []int{27}, -1},
	}
	for tidx, tc := range testCases {
		blocks := make([]*models.WSChildBlock, len(tc.FreeSizes))
		for bidx, size := range tc.FreeSizes {
			blocks[bidx] = &models.WSChildBlock{BlockSize: fmt.Sprintf("%d", size)}
		}
		choice, err := chooseBlock(blocks, tc.Size)
		if err != nil {
			if tc.ChoiceIndex != -1 {
				t.Errorf("Unexpected error for test case %d: %s", tidx, err)
				continue
			} else {
				// Expected
				continue
			}
		} else if tc.ChoiceIndex == -1 {
			t.Errorf("Expected error for test case %d but did not get one", tidx)
			continue
		}
		if choice == nil {
			t.Errorf("nil result for test case %d despite no error", tidx)
			continue
		}
		choiceIndex := -1
		for bidx, block := range blocks {
			if choice == block {
				choiceIndex = bidx
				break
			}
		}
		if choiceIndex == -1 {
			t.Errorf("got block that was not passed as input for test case %d", tidx)
		} else if choiceIndex != tc.ChoiceIndex {
			t.Errorf("got block %d but expected block %d", choiceIndex, tc.ChoiceIndex)
		}
	}
}
