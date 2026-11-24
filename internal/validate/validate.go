// Package validate is a tiny, dependency-free form validator. Errors are keyed
// by field name so templates can show them next to inputs.
//
//	v := validate.New()
//	v.Required("title", in.Title).MaxLen("title", in.Title, 200).OneOf("status", in.Status, "draft", "published")
//	if !v.OK() { return render(form with v.Errors) }
package validate

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Errors map[string]string

func (e Errors) Get(field string) string { return e[field] }
func (e Errors) Has(field string) bool   { _, ok := e[field]; return ok }

type V struct{ Errors Errors }

func New() *V { return &V{Errors: Errors{}} }

func (v *V) OK() bool { return len(v.Errors) == 0 }

// Add sets an error unless the field already has one (first error wins).
func (v *V) Add(field, msg string) *V {
	if _, ok := v.Errors[field]; !ok {
		v.Errors[field] = msg
	}
	return v
}

func (v *V) Check(ok bool, field, msg string) *V {
	if !ok {
		v.Add(field, msg)
	}
	return v
}

func (v *V) Required(field, val string) *V {
	return v.Check(strings.TrimSpace(val) != "", field, "This field is required.")
}

func (v *V) MaxLen(field, val string, n int) *V {
	return v.Check(utf8.RuneCountInString(val) <= n, field, "Too long.")
}

func (v *V) MinLen(field, val string, n int) *V {
	return v.Check(utf8.RuneCountInString(val) >= n, field, "Too short.")
}

func (v *V) Email(field, val string) *V {
	return v.Check(IsEmail(val), field, "Enter a valid email address.")
}

func (v *V) Phone(field, val string) *V {
	return v.Check(phoneRE.MatchString(val), field, "Enter a phone number in international format, e.g. +447700900123.")
}

func (v *V) OneOf(field, val string, allowed ...string) *V {
	for _, a := range allowed {
		if val == a {
			return v
		}
	}
	return v.Add(field, "Choose a valid option.")
}

var (
	phoneRE = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	uuidRE  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// IsUUID checks ids from URLs before they reach SQL (avoids 500s on garbage input).
func IsUUID(s string) bool { return uuidRE.MatchString(s) }

// IsEmail accepts a plain address (no display name) of sane length.
func IsEmail(s string) bool {
	if len(s) > 254 || strings.ContainsAny(s, " <>") {
		return false
	}
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && strings.Contains(s[strings.LastIndex(s, "@"):], ".")
}
