package templates

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

type CertInfo struct {
	Domain      string
	Issuer      string
	Expires     time.Time
	DaysRemains int
}

type DailySection struct {
	Healthy  []CertInfo
	Warnings []CertInfo
	Critical []CertInfo
}

type WeeklyReport struct {
	TotalDomains int
	Healthy      int
	Warning      int
	Expired      int
}

func ImmediateAlert(c CertInfo) (subject, body string) {
	domain := sanitizeField(c.Domain)
	subject = fmt.Sprintf("[Critical] %s expires in %d days", domain, c.DaysRemains)
	body = fmt.Sprintf(`Certificate Warning

Domain:
%s

Issuer:
%s

Expires:
%s

Days Remaining:
%d`, c.Domain, c.Issuer, c.Expires.Format("2006-01-02"), c.DaysRemains)
	return subject, body
}

func DailyDigest(date time.Time, section DailySection) (subject, body string) {
	subject = fmt.Sprintf("Daily Certificate Report - %s", date.Format("2006-01-02"))
	var b strings.Builder
	b.WriteString("Healthy Certificates\n")
	b.WriteString("--------------------\n\n")
	for _, c := range section.Healthy {
		b.WriteString(fmt.Sprintf("%s\nExpires in %d days\n\n", c.Domain, c.DaysRemains))
	}
	b.WriteString("Warnings\n")
	b.WriteString("--------\n\n")
	for _, c := range section.Warnings {
		b.WriteString(fmt.Sprintf("%s\nExpires in %d days\n\n", c.Domain, c.DaysRemains))
	}
	b.WriteString("Critical\n")
	b.WriteString("--------\n\n")
	for _, c := range section.Critical {
		b.WriteString(fmt.Sprintf("%s\nExpired %d days ago\n\n", c.Domain, -c.DaysRemains))
	}
	return subject, b.String()
}

func sanitizeField(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' || !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, s)
}

func WeeklyReportDigest(report WeeklyReport) (subject, body string) {
	subject = fmt.Sprintf("Certificate Summary - %s", time.Now().Format("2006-01-02"))
	body = fmt.Sprintf(`Certificate Summary

Total Domains
%d

Healthy
%d

Warning
%d

Expired
%d`, report.TotalDomains, report.Healthy, report.Warning, report.Expired)
	return subject, body
}
