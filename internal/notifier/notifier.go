package notifier

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/araujofrancisco/certwatch/internal/config"
)

const (
	dialTimeout = 10 * time.Second
	sendTimeout = 30 * time.Second
)

type Notifier struct {
	smtpCfg  config.SMTPConfig
	tlsCfg   *tls.Config
	profiles []config.ProfileConfig
}

func New(cfg config.NotificationsConfig) *Notifier {
	return &Notifier{
		smtpCfg:  cfg.SMTP,
		tlsCfg:   &tls.Config{ServerName: cfg.SMTP.Host},
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
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("email send cancelled: %w", err)
	}

	// SMTP requires CRLF line endings in DATA; templates use bare LF.
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"UTF-8\"\r\n\r\n%s",
		n.smtpCfg.From, strings.Join(to, ", "), subject, time.Now().Format(time.RFC1123Z), body)

	host := n.smtpCfg.Host
	addr := fmt.Sprintf("%s:%d", host, n.smtpCfg.Port)

	var auth smtp.Auth
	if n.smtpCfg.Username != "" {
		auth = smtp.PlainAuth("", n.smtpCfg.Username, n.smtpCfg.Password, host)
	}

	dialer := net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()

	// Bound all subsequent I/O (AUTH/MAIL/DATA) by both the send timeout and
	// the context, so cancelling ctx aborts an in-flight send instead of
	// letting it run to completion.
	deadline := time.Now().Add(sendTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set deadline: %w", err)
	}
	ioDone := make(chan struct{})
	defer close(ioDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now()) // unblock pending reads/writes
		case <-ioDone:
		}
	}()

	tlsCfg := n.tlsCfg
	var client *smtp.Client
	if n.smtpCfg.ForceTLS {
		tconn := tls.Client(conn, tlsCfg)
		if err := tconn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("tls handshake: %w", err)
		}
		client, err = smtp.NewClient(tconn, host)
	} else {
		client, err = smtp.NewClient(conn, host)
		if err == nil {
			// Opportunistic STARTTLS (port 587). PlainAuth is only safe after TLS.
			if ok, _ := client.Extension("STARTTLS"); ok {
				if err := client.StartTLS(tlsCfg); err != nil {
					return fmt.Errorf("starttls: %w", err)
				}
			} else if auth != nil {
				hostOnly := host == "localhost" || host == "127.0.0.1"
				if !hostOnly {
					return fmt.Errorf("server %s does not support STARTTLS; refusing to send credentials in cleartext", host)
				}
			}
		}
	}
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	if err := client.Mail(n.smtpCfg.From); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("rcpt %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		w.Close()
		return fmt.Errorf("write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close data: %w", err)
	}
	if err := client.Quit(); err != nil {
		slog.Warn("smtp quit failed", "err", err)
	}
	slog.Info("email sent", "subject", subject, "to", to)
	return nil
}
