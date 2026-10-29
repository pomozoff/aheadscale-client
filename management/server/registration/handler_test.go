package registration

import (
	"context"
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
