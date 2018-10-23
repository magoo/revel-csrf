// Package csrf is a synchronizer Token Pattern implementation.
//
// See [OWASP] https://www.owasp.org/index.php/Cross-Site_Request_Forgery_(CSRF)_Prevention_Cheat_Sheet
package csrf

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"regexp"

	"github.com/revel/revel"
)

const (
	cookieName = "csrf_token"
	fieldName  = "csrf_token"
	headerName = "X-CSRF-Token"
)

var (
	errNoReferer  = "REVEL_CSRF: A secure request contained no Referer or its value was malformed."
	errBadReferer = "REVEL_CSRF: Same-origin policy failure."
	errBadToken   = "REVEL_CSRF: tokens mismatch."
	safeMethods   = regexp.MustCompile("^(GET|HEAD|OPTIONS|TRACE|WS)$")
)

// CSRFFilter implements the CSRF filter.
var CSRFFilter = func(c *revel.Controller, fc []revel.Filter) {
	r := c.Request.In.GetRaw().(*http.Request)

	// [OWASP]; General Recommendation: Synchronizer Token Pattern:
	// CSRF tokens must be associated with the user's current session.
	tokenCookie, found := c.Session[cookieName]
	realToken := ""
	if !found {
		realToken = generateNewToken(c)
	} else {
		realToken = tokenCookie
		//revel.AppLog.Debug("REVEL-CSRF: Session's token.",  "token", realToken)
		if len(realToken) != lengthCSRFToken {
			// Wrong length; token has either been tampered with, we're migrating
			// onto a new algorithm for generating tokens, or a new session has
			// been initiated. In any case, a new token is generated and the
			// error will be detected later.
			revel.AppLog.Debug("REVEL_CSRF: Bad token length.", "found", len(realToken), "expected", lengthCSRFToken)
			realToken = generateNewToken(c)
		}
	}

	c.ViewArgs[fieldName] = realToken

	// See http://en.wikipedia.org/wiki/Hypertext_Transfer_Protocol#Safe_methods
	unsafeMethod := !safeMethods.MatchString(r.Method)
	if unsafeMethod && !IsExempted(r.URL.Path) {
		//Replace with logrus
		//revel.AppLog.Debug("REVEL-CSRF: Processing unsafe method.", "method", r.Method)
		if r.URL.Scheme == "https" {
			// See [OWASP]; Checking the Referer Header.
			referer, err := url.Parse(r.Header.Get("Referer"))
			if err != nil || referer.String() == "" {
				// Parse error or empty referer.
				c.Result = c.Forbidden(errNoReferer)
				return
			}
			// See [OWASP]; Checking the Origin Header.
			if !sameOrigin(referer, r.URL) {
				c.Result = c.Forbidden(errBadReferer)
				return
			}
		}

		sentToken := ""
		if ajaxSupport := revel.Config.BoolDefault("csrf.ajax", false); ajaxSupport {
			// Accept CSRF token in the custom HTTP header X-CSRF-Token, for ease
			// of use with popular JavaScript toolkits which allow insertion of
			// custom headers into all AJAX requests.
			// See http://erlend.oftedal.no/blog/?blogid=118
			sentToken = r.Header.Get(headerName)
		}
		if sentToken == "" {
			// Get CSRF token from form.
			sentToken = c.Params.Get(fieldName)
		}
		//revel.AppLog.Debug("REVEL-CSRF: Token received from client.", "token", sentToken)

		if len(sentToken) != len(realToken) {
			revel.AppLog.Debug("REVEL-CSRF: Bad Token received from client:", "token", sentToken)
			c.Result = c.Forbidden(errBadToken)
			return
		}
		comparison := subtle.ConstantTimeCompare([]byte(sentToken), []byte(realToken))
		if comparison != 1 {
			revel.AppLog.Debug("REVEL-CSRF: Bad Token received from client.", "token", sentToken)
			c.Result = c.Forbidden(errBadToken)
			return
		}
		//revel.AppLog.Debug("REVEL-CSRF: Token successfully checked.")
	}

	fc[0](c, fc[1:])
}

// See http://en.wikipedia.org/wiki/Same-origin_policy
func sameOrigin(u1, u2 *url.URL) bool {
	return (u1.Scheme == u2.Scheme && u1.Host == u2.Host)
}
