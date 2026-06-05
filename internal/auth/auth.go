package auth

import (
	"crypto/subtle"
	"net/http"
)

type Credentials struct {
	User     string
	Password string
}

func (c Credentials) IsDefault() bool {
	return c.User == "admin" && c.Password == "admin"
}

func Middleware(creds Credentials, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || !same(user, creds.User) || !same(pass, creds.Password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="iospublisher", charset="UTF-8"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func same(got, want string) bool {
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
