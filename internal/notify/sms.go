package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SMS struct {
	To   string // E.164, e.g. +447700900123
	Body string
}

type SMSSender interface {
	Send(ctx context.Context, m SMS) error
}

// Cost tip: SMS is the most expensive channel (and SIM-swap prone). Prefer
// email codes/magic links and passkeys; use SMS only where the product needs a
// verified phone number.

// LogSMS prints messages instead of sending (development / tests).
type LogSMS struct{ Log *slog.Logger }

func (l LogSMS) Send(_ context.Context, m SMS) error {
	l.Log.Info("sms (log driver, not sent)", "to", m.To, "body", m.Body)
	return nil
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// Twilio Messages API.
type Twilio struct{ SID, Token, From string }

func (t Twilio) Send(ctx context.Context, m SMS) error {
	form := url.Values{"To": {m.To}, "From": {t.From}, "Body": {m.Body}}
	u := "https://api.twilio.com/2010-04-01/Accounts/" + url.PathEscape(t.SID) + "/Messages.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(t.SID, t.Token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return doJSON(req, http.StatusCreated, http.StatusOK)
}

// Vonage (Nexmo) SMS API.
type Vonage struct{ Key, Secret, From string }

func (v Vonage) Send(ctx context.Context, m SMS) error {
	body, _ := json.Marshal(map[string]string{
		"api_key": v.Key, "api_secret": v.Secret, "from": v.From,
		"to": strings.TrimPrefix(m.To, "+"), "text": m.Body,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://rest.nexmo.com/sms/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var out struct {
		Messages []struct {
			Status    string `json:"status"`
			ErrorText string `json:"error-text"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&out); err != nil {
		return err
	}
	if len(out.Messages) == 0 || out.Messages[0].Status != "0" {
		if len(out.Messages) > 0 {
			return fmt.Errorf("notify: vonage: %s", out.Messages[0].ErrorText)
		}
		return fmt.Errorf("notify: vonage: empty response")
	}
	return nil
}

func doJSON(req *http.Request, ok ...int) error {
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	for _, c := range ok {
		if res.StatusCode == c {
			return nil
		}
	}
	b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	return fmt.Errorf("notify: %s returned %d: %s", req.URL.Host, res.StatusCode, b)
}

// NewSMS picks a provider from config values.
func NewSMS(provider, from, twSID, twToken, vgKey, vgSecret string, log *slog.Logger) SMSSender {
	switch provider {
	case "twilio":
		return Twilio{SID: twSID, Token: twToken, From: from}
	case "vonage":
		return Vonage{Key: vgKey, Secret: vgSecret, From: from}
	default:
		return LogSMS{Log: log}
	}
}
