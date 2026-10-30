package registration

import (
	"bytes"
	_ "embed"
	"fmt"
	htmltemplate "html/template"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	texttemplate "text/template"
	"time"
)

//go:embed mail/verification.html
var verificationEmailHTMLSource string

//go:embed mail/verification.txt
var verificationEmailTextSource string

var verificationEmailHTML = htmltemplate.Must(htmltemplate.New("verification").Parse(verificationEmailHTMLSource))
var verificationEmailText = texttemplate.Must(texttemplate.New("verification").Parse(verificationEmailTextSource))

type verificationEmailData struct {
	Email    string
	Link     string
	Lifetime string
}

func buildVerificationMessage(from *mail.Address, to, link string, ttl time.Duration) ([]byte, error) {
	u, err := url.Parse(link)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || strings.ContainsAny(link, "\r\n") {
		return nil, ErrInvalid
	}
	if ttl < 0 {
		return nil, ErrInvalid
	}
	if ttl == 0 {
		ttl = 30 * time.Minute
	}
	data := verificationEmailData{Email: to, Link: link, Lifetime: verificationLifetime(ttl)}
	var message bytes.Buffer
	writer := multipart.NewWriter(&message)
	message.WriteString("From: " + from.String() + "\r\n")
	message.WriteString("To: " + to + "\r\n")
	message.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", "Подтвердите email — Aheadscale") + "\r\n")
	message.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	message.WriteString("Message-ID: <" + writer.Boundary() + "@aheadscale.ru>\r\n")
	message.WriteString("Auto-Submitted: auto-generated\r\nMIME-Version: 1.0\r\n")
	message.WriteString("Content-Type: " + mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": writer.Boundary()}) + "\r\n\r\n")
	for _, alternative := range []struct {
		kind   string
		render func(io.Writer, any) error
	}{{"text/plain", verificationEmailText.Execute}, {"text/html", verificationEmailHTML.Execute}} {
		part, err := writer.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {alternative.kind + "; charset=UTF-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		encoded := quotedprintable.NewWriter(part)
		if err := alternative.render(encoded, data); err != nil {
			return nil, err
		}
		if err := encoded.Close(); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return message.Bytes(), nil
}

func verificationLifetime(ttl time.Duration) string {
	for _, unit := range []struct {
		size  time.Duration
		forms [3]string
	}{
		{time.Hour, [3]string{"час", "часа", "часов"}},
		{time.Minute, [3]string{"минута", "минуты", "минут"}},
		{time.Second, [3]string{"секунда", "секунды", "секунд"}},
	} {
		if ttl%unit.size != 0 {
			continue
		}
		count := int(ttl / unit.size)
		form := unit.forms[2]
		if count%100 < 11 || count%100 > 14 {
			if count%10 == 1 {
				form = unit.forms[0]
			} else if count%10 >= 2 && count%10 <= 4 {
				form = unit.forms[1]
			}
		}
		return fmt.Sprintf("%d %s", count, form)
	}
	return strconv.FormatFloat(ttl.Seconds(), 'f', -1, 64) + " с"
}
