package mail

import "testing"

func FuzzNewAddress(f *testing.F) {
	f.Add("name@example.com")
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = NewAddress(s)
	})
}
