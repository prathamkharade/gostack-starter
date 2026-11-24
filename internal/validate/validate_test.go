package validate

import "testing"

func TestValidator(t *testing.T) {
	v := New().
		Required("title", " ").
		MaxLen("title", "x", 10).
		Email("email", "not-an-email").
		Phone("phone", "+447700900123").
		OneOf("status", "evil", "draft", "published")
	if v.OK() {
		t.Fatal("expected errors")
	}
	if v.Errors.Get("title") != "This field is required." {
		t.Fatal("first error must win")
	}
	if !v.Errors.Has("email") || v.Errors.Has("phone") || !v.Errors.Has("status") {
		t.Fatalf("unexpected errors %v", v.Errors)
	}
}

func TestIsEmail(t *testing.T) {
	good := []string{"a@b.co", "jane.doe+tag@example.com"}
	bad := []string{"", "a@b", "Jane <a@b.co>", "a b@c.com", "@b.co"}
	for _, s := range good {
		if !IsEmail(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range bad {
		if IsEmail(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestIsUUID(t *testing.T) {
	if !IsUUID("019a5c2e-7b1d-7c3e-9f00-0123456789ab") || IsUUID("1 OR 1=1") || IsUUID("") {
		t.Fatal("IsUUID")
	}
}
