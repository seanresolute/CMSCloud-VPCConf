package ipcontrol

import (
	"testing"

	"github.com/CMSgov/CMS-AWS-West-Network-Architecture/vpc-automation/database"
)

type devNullLogger int

func (devNullLogger) Log(string, ...interface{}) {}

var devNull devNullLogger

func TestChooseBlockSize(t *testing.T) {
	type testCase struct {
		privateSize, numPrivate, publicSize, numPublic int
		privateBlockSize, publicBlockSize              int
	}
	testCases := []*testCase{
		{
			25, 2, 25, 2,
			23, -1,
		},
		{
			25, 2, 25, 0,
			24, -1,
		},
		{
			25, 0, 25, 2,
			24, -1,
		},
		{
			25, 1, 24, 1,
			25, 24,
		},
		{
			24, 1, 25, 1,
			24, 25,
		},
		{
			24, 1, 24, 1,
			23, -1,
		},
		{
			24, 2, 24, 2,
			22, -1,
		},
		{
			24, 3, 24, 1,
			22, -1,
		},
		{
			24, 3, 23, 1,
			22, 23,
		},
		{
			20, 6, 20, 2,
			17, -1,
		},
		{
			20, 6, 20, 3,
			17, 18,
		},
		{
			21, 1, 20, 3,
			18, -1,
		},
		{
			21, 2, 20, 3,
			18, -1,
		},
		{
			21, 3, 20, 3,
			19, 18,
		},
	}
	for tidx, tc := range testCases {
		cfg := &database.AllocateConfig{
			PrivateSize:       tc.privateSize,
			NumPrivateSubnets: tc.numPrivate,
			PublicSize:        tc.publicSize,
			NumPublicSubnets:  tc.numPublic,
		}
		private, public := chooseBlockSize(cfg, devNull)
		if private != tc.privateBlockSize || public != tc.publicBlockSize {
			t.Errorf("Test case %d: got %d/%d but expected %d/%d", tidx, private, public, tc.privateBlockSize, tc.publicBlockSize)
		}
	}
}
