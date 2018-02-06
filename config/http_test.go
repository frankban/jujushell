package config_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestHTTP(t *testing.T) {
	c := qt.New(t)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "boo")
	}))
	defer s.Close()
	resp, err := http.DefaultClient.Get(s.URL)
	c.Assert(err, qt.Equals, nil)
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, qt.Equals, nil)
	c.Assert(string(b), qt.Equals, "boo")
}
