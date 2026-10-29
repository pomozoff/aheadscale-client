// Package registration adds verified signup to the existing Dex browser flow.
package registration

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/netbirdio/netbird/management/server/idp"
	"golang.org/x/time/rate"
)

//go:embed web/verification.html
var verificationHTML []byte
//go:embed web/verification.js
var verificationJS []byte
//go:embed web/verification.css
var verificationCSS []byte

// A configured Service is mandatory; old callers cannot enable unverified signup.
type identityStore interface {
	CreateUserWithPassword(context.Context,string,string,string) (*idp.UserData,error)
	DeleteUser(context.Context,string) error
}

func NewHandler(issuer string,_ identityStore,_ func(context.Context,*idp.UserData)error,next http.Handler,services ...*Service) http.Handler {
	parsed,_:=url.Parse(issuer);origin:="";secure:=false
	if parsed!=nil&&parsed.Host!="" {origin=parsed.Scheme+"://"+parsed.Host;secure=parsed.Scheme=="https"}
	var service *Service;if len(services)==1 {service=services[0]}
	limit:=rate.NewLimiter(rate.Every(time.Second),20);working:=make(chan struct{},2)
	cookieName:="as_reg_browser";if secure {cookieName="__Host-as-reg-browser"}
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		binding:="";if c,err:=r.Cookie(cookieName);err==nil&&len(c.Value)==43 {binding=c.Value}
		if r.Method==http.MethodGet&&strings.HasPrefix(r.URL.Path,"/oauth2/auth/")&&binding=="" {
			if value,err:=randomSecret();err==nil {http.SetCookie(w,&http.Cookie{Name:cookieName,Value:value,Path:"/",HttpOnly:true,Secure:secure,SameSite:http.SameSiteLaxMode,MaxAge:86400})}
		}
		switch r.URL.Path {
		case "/oauth2/verify-email","/oauth2/registration.js","/oauth2/registration.css":
			securityHeaders(w)
			if r.Method!=http.MethodGet {w.Header().Set("Allow","GET");http.Error(w,"Method not allowed",405);return}
			if r.URL.Path=="/oauth2/registration.js" {w.Header().Set("Content-Type","text/javascript; charset=utf-8");_,_ = w.Write(verificationJS)} else if r.URL.Path=="/oauth2/registration.css" {w.Header().Set("Content-Type","text/css; charset=utf-8");_,_ = w.Write(verificationCSS)} else {w.Header().Set("Content-Type","text/html; charset=utf-8");_,_ = w.Write(verificationHTML)}
			return
		case "/oauth2/signup","/oauth2/signup/inspect","/oauth2/signup/complete":
		default:next.ServeHTTP(w,r);return
		}
		securityHeaders(w)
		if r.Method!=http.MethodPost {w.Header().Set("Allow","POST");http.Error(w,"Method not allowed",405);return}
		if origin==""||r.Header.Get("Origin")!=origin {http.Error(w,"Forbidden",403);return}
		kind,_,err:=mime.ParseMediaType(r.Header.Get("Content-Type"));if err!=nil||kind!="application/json" {http.Error(w,"Expected JSON",415);return}
		if !limit.Allow(){w.Header().Set("Retry-After","60");http.Error(w,"Please try again shortly",429);return}
		select {case working<-struct{}{}:defer func(){<-working}();default:http.Error(w,"Please try again shortly",429);return}
		if service==nil {http.Error(w,"Email registration is temporarily unavailable. Please try again later.",503);return}
		var output any;status:=http.StatusOK
		switch r.URL.Path {
		case "/oauth2/signup":
			var input struct {Email string `json:"email"`;Continuation string `json:"continuation"`};if !decode(w,r,&input){return}
			if binding=="" {binding,err=randomSecret();if err!=nil {http.Error(w,"Registration unavailable",503);return};http.SetCookie(w,&http.Cookie{Name:cookieName,Value:binding,Path:"/",HttpOnly:true,Secure:secure,SameSite:http.SameSiteLaxMode,MaxAge:86400})}
			source,_,splitErr:=net.SplitHostPort(r.RemoteAddr);if splitErr!=nil {source=r.RemoteAddr}
			err=service.Start(r.Context(),input.Email,binding,input.Continuation,source)
			status=http.StatusAccepted;output=map[string]any{"pending":true,"retryAfterSeconds":int(service.config.ResendAfter.Seconds()+0.999)}
		case "/oauth2/signup/inspect":
			var input struct {Token string `json:"token"`};if !decode(w,r,&input){return}
			var row PendingRegistration
			if len(input.Token)!=43 {err=ErrToken} else {err=service.db.WithContext(r.Context()).Where("token_hash = ? AND expires_at > ? AND state = ?",digest(input.Token),service.now().UTC(),"pending").Take(&row).Error}
			if err!=nil {err=ErrToken};output=map[string]string{"email":row.Email}
		case "/oauth2/signup/complete":
			var input struct {Token string `json:"token"`;Password string `json:"password"`};if !decode(w,r,&input){return}
			if len(input.Password)<8||len(input.Password)>72 {http.Error(w,"Enter a password of 8–72 bytes",400);return}
			output,err=service.Complete(r.Context(),input.Token,input.Password,binding)
		}
		if err!=nil {
			switch {case errors.Is(err,ErrRateLimited):w.Header().Set("Retry-After",strconv.Itoa(int(service.config.ResendAfter.Seconds()+0.999)));http.Error(w,"Please wait before requesting another email",429)
			case errors.Is(err,ErrInvalid):http.Error(w,"Enter a valid email and use a current sign-in page",400)
			case errors.Is(err,ErrToken):http.Error(w,"Invalid or expired link. Request a new email, or sign in if already registered.",400)
			case errors.Is(err,ErrDelivery):http.Error(w,"Email could not be sent. Please try again shortly.",503)
			default:http.Error(w,"Registration could not be completed. Please retry this link shortly.",503)};return
		}
		w.Header().Set("Content-Type","application/json");w.WriteHeader(status);_ = json.NewEncoder(w).Encode(output)
	})
}
func securityHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control","no-store");w.Header().Set("X-Content-Type-Options","nosniff");w.Header().Set("Referrer-Policy","no-referrer")
	w.Header().Set("Content-Security-Policy","default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
}
func decode(w http.ResponseWriter,r *http.Request,value any) bool {
	d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,4096));d.DisallowUnknownFields()
	if d.Decode(value)!=nil||d.Decode(new(any))!=io.EOF {http.Error(w,"Invalid registration details",400);return false};return true
}
