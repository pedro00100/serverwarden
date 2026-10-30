package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

var emailType = &Type{
	ID: "email", Name: "Email (SMTP)",
	Fields: []Field{
		{Key: "host", Label: "SMTP server", Kind: "text", Required: true},
		{Key: "port", Label: "Port", Kind: "number", Default: "587"},
		{Key: "security", Label: "Security", Kind: "select", Default: "starttls", Options: []string{"starttls", "tls", "none"}},
		{Key: "username", Label: "Username", Kind: "text"},
		{Key: "password", Label: "Password", Kind: "text", Secret: true},
		{Key: "from", Label: "From", Kind: "text", Required: true},
		{Key: "to", Label: "To", Kind: "text", Required: true, Help: "Comma-separated addresses."},
	},
	send: sendEmail,
}

// headerSafe drops line breaks so no value can inject extra headers.
func headerSafe(s string) string { return strings.NewReplacer("\r", "", "\n", "").Replace(s) }

func sendEmail(ctx context.Context, s map[string]string, m Message) error {
	host := s["host"]
	addr := net.JoinHostPort(host, s["port"])
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	var conn net.Conn
	var err error
	if s["security"] == "tls" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if s["security"] == "starttls" {
		if err := c.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if s["username"] != "" {
		// PlainAuth refuses to send the password over an unencrypted remote connection.
		if err := c.Auth(smtp.PlainAuth("", s["username"], s["password"], host)); err != nil {
			return err
		}
	}
	from := headerSafe(s["from"])
	var to []string
	for _, a := range strings.Split(s["to"], ",") {
		if a = headerSafe(strings.TrimSpace(a)); a != "" {
			to = append(to, a)
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, a := range to {
		if err := c.Rcpt(a); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n",
		from, strings.Join(to, ", "), mime.QEncoding.Encode("utf-8", headerSafe(m.Title)), time.Now().Format(time.RFC1123Z),
		strings.ReplaceAll(m.Body, "\n", "\r\n"))
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
