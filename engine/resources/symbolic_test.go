package resources

import (
	"testing"
)

func Test_mode(t *testing.T) {
	testCases := []struct {
		input string
		want  string
		err   error
	}{
		{input: "ug=r,go=r,ua=x", want: "0555"},
		{input: "uu=rw,ug=rw,gg=xr", want: "0670"},
		{input: "u=r", want: "0400"},
		{input: "g=rw", want: "0060"},
		{input: "o=rwx", want: "0007"},
		{input: "u=r", want: "0400"},
		{input: "ugo=r,g=w,o=w", want: "0466"},
		{input: "u=r,g=w,o=x", want: "0421"},
		{input: "ug=rw,o=rx", want: "0665"},
		{input: "g=r,ugo=x,a=w", want: "0373"},

		// currently failing tests
		// {input: "o=args", err: fmt.Errorf("permission setting failed")},
		// {input: "o=arq", err: fmt.Errorf("permission setting failed")},
	}
	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := symbolicMode(tc.input)
			if err != tc.err {
				t.Errorf("error mismatch. Actual: %v, Expected: %v", err, tc.err)
			}
			if result != tc.want {
				t.Errorf("permission setting failed. Actual: %v, expected: %v", result, tc.want)
			}
			if result == tc.want {
				t.Log("permission successfully set")
			}
		})
	}
}
