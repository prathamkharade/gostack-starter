package pages

import (
	"strings"

	"app/internal/auth"
	"app/internal/validate"
)

// LoginState drives the multi-step sign-in card.
type LoginState struct {
	Step   string // email | code | password
	Email  string
	Next   string
	Errors validate.Errors
}

// AccountData is everything the account page shows.
type AccountData struct {
	User           *auth.User
	Sessions       []auth.ActiveSession
	Consents       map[string]bool
	CurrentSession string
	Errors         validate.Errors
}

// shortUA turns a user-agent into "Firefox on Windows" for the sessions list.
func shortUA(ua string) string {
	browser, os := "Browser", "unknown device"
	for _, b := range []struct{ key, name string }{{"Edg/", "Edge"}, {"OPR/", "Opera"}, {"Firefox/", "Firefox"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"}} {
		if strings.Contains(ua, b.key) {
			browser = b.name
			break
		}
	}
	for _, o := range []struct{ key, name string }{{"Android", "Android"}, {"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Windows", "Windows"}, {"Mac OS", "macOS"}, {"Linux", "Linux"}} {
		if strings.Contains(ua, o.key) {
			os = o.name
			break
		}
	}
	return browser + " on " + os
}
