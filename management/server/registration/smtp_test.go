package registration

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestSMTPDeliveryUsesTLSAndContainsOnlyVerificationLink(t *testing.T) {
	// httptest provides a real test certificate; the SMTP protocol below uses it.
	certServer:=httptest.NewTLSServer(nil);cert:=certServer.TLS.Certificates[0];root:=certServer.Certificate();certServer.Close()
	listener,err:=tls.Listen("tcp","127.0.0.1:0",&tls.Config{Certificates:[]tls.Certificate{cert},MinVersion:tls.VersionTLS12});if err!=nil{t.Fatal(err)};defer listener.Close()
	messages:=make(chan string,1);failures:=make(chan error,1)
	go func(){
		conn,err:=listener.Accept();if err!=nil{failures<-err;return};defer conn.Close();_ = conn.SetDeadline(time.Now().Add(5*time.Second))
		r:=textproto.NewReader(bufio.NewReader(conn));w:=bufio.NewWriter(conn)
		reply:=func(value string){_,_ = w.WriteString(value+"\r\n");_ = w.Flush()};reply("220 test SMTP")
		for {
			line,e:=r.ReadLine();if e!=nil{failures<-e;return}
			switch {
			case strings.HasPrefix(line,"EHLO"):reply("250-test\r\n250 AUTH PLAIN")
			case strings.HasPrefix(line,"AUTH PLAIN "):reply("235 OK")
			case line=="MAIL FROM:<noreply@aheadscale.ru>"||line=="RCPT TO:<owner@example.test>":reply("250 OK")
			case line=="DATA":reply("354 Ready");body,e:=r.ReadDotBytes();if e!=nil{failures<-e;return};messages<-string(body);reply("250 Accepted")
			case line=="QUIT":reply("221 Bye");return
			default:failures<-errors.New("unexpected SMTP command");return
			}
		}
	}()
	roots:=x509.NewCertPool();roots.AddCert(root);_,port,_:=net.SplitHostPort(listener.Addr().String())
	m:=&SMTPMailer{Host:"127.0.0.1",Port:port,Mode:"implicit",Username:"ci",Password:"smtp-only-secret",From:"Aheadscale <noreply@aheadscale.ru>",tlsConfig:&tls.Config{RootCAs:roots}}
	if err=m.Send(context.Background(),"owner@example.test","https://aheadscale.ru/oauth2/verify-email#one-time-link");err!=nil{t.Fatal(err)}
	select {case body:=<-messages:
		if !strings.Contains(body,"https://aheadscale.ru/oauth2/verify-email#one-time-link")||strings.Contains(body,"smtp-only-secret")||!strings.Contains(body,"From: "){t.Fatal("incorrect verification email")}
	case err:=<-failures:t.Fatal(err)
	case <-time.After(5*time.Second):t.Fatal("SMTP message not received")}
}

func TestSMTPNeverAuthenticatesWithoutRequiredSTARTTLS(t *testing.T) {
	l,err:=net.Listen("tcp","127.0.0.1:0");if err!=nil{t.Fatal(err)};defer l.Close()
	commands:=make(chan string,1)
	go func(){c,e:=l.Accept();if e!=nil{return};defer c.Close();_ = c.SetDeadline(time.Now().Add(3*time.Second))
		_,_ = c.Write([]byte("220 plaintext test\r\n"));r:=bufio.NewReader(c);_,_ = r.ReadString('\n');_,_ = c.Write([]byte("250 plaintext\r\n"));line,_:=r.ReadString('\n');commands<-line
	}()
	_,port,_:=net.SplitHostPort(l.Addr().String());m:=&SMTPMailer{Host:"127.0.0.1",Port:port,Mode:"starttls",Username:"ci",Password:"must-not-leak",From:"Aheadscale <noreply@aheadscale.ru>"}
	if err=m.Send(context.Background(),"owner@example.test","https://aheadscale.ru/oauth2/verify-email#token");!errors.Is(err,ErrDelivery){t.Fatalf("plaintext SMTP not rejected: %v",err)}
	if line:=<-commands;strings.Contains(line,"AUTH")||strings.Contains(line,"must-not-leak"){t.Fatal("credentials sent without TLS")}
}
