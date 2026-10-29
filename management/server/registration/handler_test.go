package registration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netbirdio/netbird/management/server/idp"
)

type recordingIdentity struct{ created int }

func (i *recordingIdentity) CreateUserWithPassword(context.Context, string, string, string) (*idp.UserData, error) {
	i.created++
	return &idp.UserData{ID: "unverified", Email: "owner@example.test"}, nil
}

func TestHTTPVerificationGETDoesNotConsumeAndOriginIsRequired(t *testing.T) {
	s,mail,_,_:=newTestService(t)
	if err:=s.Start(context.Background(),"owner@example.test","browser-a","","192.0.2.1");err!=nil{t.Fatal(err)}
	token:=tokenFromMail(t,mail)
	h:=NewHandler("https://aheadscale.ru/oauth2",&recordingIdentity{},func(context.Context,*idp.UserData)error{return nil},http.NotFoundHandler(),s)
	page:=httptest.NewRecorder();h.ServeHTTP(page,httptest.NewRequest("GET","/oauth2/verify-email",nil))
	if page.Code!=200||page.Header().Get("Referrer-Policy")!="no-referrer"{t.Fatal("verification page unavailable")}
	for _,c:=range []struct{origin string;want int}{{"https://evil.example",403},{"https://aheadscale.ru",200},{"https://aheadscale.ru",400}} {
		body,_:=json.Marshal(map[string]string{"token":token,"password":"correct-horse-123"})
		r:=httptest.NewRequest("POST","/oauth2/signup/complete",strings.NewReader(string(body)));r.Header.Set("Origin",c.origin);r.Header.Set("Content-Type","application/json")
		w:=httptest.NewRecorder();h.ServeHTTP(w,r);if w.Code!=c.want{t.Fatalf("origin/replay: HTTP %d, want %d",w.Code,c.want)}
	}
}

func (*recordingIdentity) DeleteUser(context.Context, string) error { return nil }

// A client using the previous payload must not bypass email proof.
func TestLegacySignupCannotActivateAccount(t *testing.T) {
	identities := &recordingIdentity{}
	provisioned := false
	handler := NewHandler("https://aheadscale.ru/oauth2", identities, func(context.Context, *idp.UserData) error {
		provisioned = true
		return nil
	}, http.NotFoundHandler())
	r := httptest.NewRequest(http.MethodPost, "/oauth2/signup", strings.NewReader(`{"email":"owner@example.test","password":"correct-horse-123"}`))
	r.Header.Set("Origin", "https://aheadscale.ru")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code < 400 || identities.created != 0 || provisioned {
		t.Fatalf("unverified legacy signup activated an account: HTTP %d, identities %d, provisioned %t", w.Code, identities.created, provisioned)
	}
}
