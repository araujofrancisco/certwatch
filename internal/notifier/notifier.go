package notifier

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"

	"github.com/araujofrancisco/certwatch/internal/config"
)

type Notifier struct {
	smtpCfg config.SMTPConfig
	profiles []config.ProfileConfig
}

func New(cfg config.NotificationsConfig) *Notifier {
	return &Notifier{
		smtpCfg:  cfg.SMTP,
		profiles: cfg.Profiles,
	}
}

func (n *Notifier) Profiles() []config.ProfileConfig {
	return n.profiles
}

func (n *Notifier) SendEmail(ctx context.Context, to []string, subject, body string) error {
	if n.smtpCfg.Host == "" {
		slog.Warn("smtp not configured, skipping email", "subject", subject, "to", to)
		return nil
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"UTF-8\"\r\n\r\n%s",
		n.smtpCfg.From, strings.Join(to, ", "), subject, body)

	addr := fmt.Sprintf("%s:%d", n.smtpCfg.Host, n.smtpCfg.Port)

	var auth smtp.Auth
	if n.smtpCfg.Username != "" {
		auth = smtp.PlainAuth("", n.smtpCfg.Username, n.smtpCfg.Password, n.smtpCfg.Host)
	}

	if n.smtpCfg.ForceTLS {
		if err := sendMailTLS(addr, auth, n.smtpCfg.From, to, []byte(msg)); err != nil {
			return fmt.Errorf("send email (tls): %w", err)
		}
	} else {
		if err := smtp.SendMail(addr, auth, n.smtpCfg.From, to, []byte(msg)); err != nil {
			return fmt.Errorf("send email: %w", err)
		}
	}
	slog.Info("email sent", "subject", subject, "to", to)
	return nil
}

func sendMailTLS(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
	tlsCfg := &tls.Config{ServerName: addr[:strings.LastIndex(addr, ":")]}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	client, err := smtp.NewClient(conn, addr[:strings.LastIndex(addr, ":")])
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()
	if a != nil {
		if err := client.Auth(a); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	for _, addr := range to {
		if err := client.Rcpt(addr); err != nil {
			return fmt.Errorf("rcpt %s: %w", addr, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	_, err = w.Write(msg)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return w.Close()
}
