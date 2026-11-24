package notify

import (
	"bufio"
	"context"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestBuildMIME(t *testing.T) {
	from, _ := mail.ParseAddress("MyApp <no-reply@example.com>")
	to, _ := mail.ParseAddress("jane@example.com")
	e := LoginCode("MyApp", to.Address, "123456", "https://example.com/auth/verify?t=abc", 10)
	msg, err := BuildMIME(from, to, e, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	s := string(msg)
	for _, want := range []string{"multipart/alternative", "Subject: 123456 is your MyApp code", "text/plain", "text/html", "Message-ID: <"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	if _, err := BuildMIME(from, to, Email{Subject: "x\r\nBcc: evil@x.com"}, time.Now()); err == nil {
		t.Fatal("header injection must be rejected")
	}
}

// fake SMTP server: verifies the client speaks the protocol correctly.
func TestSMTPMailerAgainstFakeServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		w := func(s string) { c.Write([]byte(s + "\r\n")) }
		w("220 fake ESMTP")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					w("250 queued")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				w("250 fake")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				w("250 ok")
			case cmd == "DATA":
				inData = true
				w("354 go")
			case cmd == "QUIT":
				w("221 bye")
				got <- data.String()
				return
			default:
				w("250 ok")
			}
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	var p int
	for _, ch := range port {
		p = p*10 + int(ch-'0')
	}
	m := &SMTPMailer{Host: host, Port: p, TLS: "none", From: "App <a@example.com>"}
	if err := m.Send(context.Background(), Email{To: "b@example.com", Subject: "Hi", Text: "Hello"}); err != nil {
		t.Fatal(err)
	}
	select {
	case d := <-got:
		if !strings.Contains(d, "Subject: Hi") || !strings.Contains(d, "Hello") {
			t.Fatalf("unexpected data: %s", d)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not receive message")
	}
}
