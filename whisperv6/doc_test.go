package whisperv6

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSyncMailRequestValidate(t *testing.T) {
	testCases := []struct {
		Name  string
		Req   SyncMailRequest
		Error string
	}{
		{
			Name: "empty request is valid",
			Req:  SyncMailRequest{},
		},
		{
			Name:  "invalid Limit",
			Req:   SyncMailRequest{Limit: 1e6},
			Error: "invalid 'Limit' value, expected lower than 1000",
		},
		{
			Name:  "invalid Lower",
			Req:   SyncMailRequest{Lower: 10, Upper: 5},
			Error: "invalid 'Lower' value, can't be greater than 'Upper'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			err := tc.Req.Validate()
			if tc.Error != "" {
				assert.EqualError(t, err, tc.Error)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
