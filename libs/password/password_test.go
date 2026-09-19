package password_test

import (
	"testing"

	"grindstats/libs/password"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	h, err := password.New(password.Fast)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	encoded, err := h.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	ok, err := h.Verify("correct horse battery staple", encoded)
	if err != nil || !ok {
		t.Fatalf("Verify(correct) = %v, %v; want true, nil", ok, err)
	}

	ok, err = h.Verify("wrong password", encoded)
	if err != nil || ok {
		t.Fatalf("Verify(wrong) = %v, %v; want false, nil", ok, err)
	}
}

func TestVerifyEmptyEncodedIsNoMatch(t *testing.T) {
	h, err := password.New(password.Fast)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ok, err := h.Verify("anything", "")
	if err != nil || ok {
		t.Fatalf("Verify(empty) = %v, %v; want false, nil", ok, err)
	}
}

func TestVerifyRejectsWrongVersion(t *testing.T) {
	h, err := password.New(password.Fast)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = h.Verify("x", "$argon2id$v=18$m=8192,t=1,p=1$c2FsdA$aGFzaA")
	if err == nil {
		t.Fatal("Verify with mismatched version = nil error, want error")
	}
}
