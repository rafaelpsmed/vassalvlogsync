package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"os"
	"strings"
)

type Mailer struct {
	host     string
	port     string
	user     string
	password string
	from     string
	enabled  bool
}

func NewMailerFromEnv() *Mailer {
	host := os.Getenv("SMTP_HOST")
	m := &Mailer{
		host:     host,
		port:     envOr("SMTP_PORT", "587"),
		user:     os.Getenv("SMTP_USER"),
		password: os.Getenv("SMTP_PASSWORD"),
		from:     envOr("SMTP_FROM", os.Getenv("SMTP_USER")),
		enabled:  host != "",
	}
	return m
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (m *Mailer) SendTurnNotification(ctx context.Context, toEmail, playerName, gameName string) error {
	if !m.enabled {
		return nil
	}
	subject := fmt.Sprintf("Vassal vLog Sync — é sua vez em %s", gameName)
	body := fmt.Sprintf("Olá %s,\n\nÉ sua vez na partida \"%s\". Abra o Vassal e carregue o arquivo .vlog sincronizado.\n\n— Vassal vLog Sync\n", playerName, gameName)

	msg := strings.Join([]string{
		fmt.Sprintf("From: %s", m.from),
		fmt.Sprintf("To: %s", toEmail),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	addr := fmt.Sprintf("%s:%s", m.host, m.port)
	auth := smtp.PlainAuth("", m.user, m.password, m.host)

	if m.port == "465" {
		return m.sendTLS(addr, auth, toEmail, []byte(msg))
	}
	return smtp.SendMail(addr, auth, m.from, []string{toEmail}, []byte(msg))
}

func (m *Mailer) sendTLS(addr string, auth smtp.Auth, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: strings.Split(addr, ":")[0]})
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(m.from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}
