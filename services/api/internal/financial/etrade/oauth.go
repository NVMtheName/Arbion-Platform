package etrade

import (
	"crypto/hmac"
	"crypto/sha1" // E*TRADE mandates OAuth 1.0a HMAC-SHA1, not a password hash.
	"encoding/base64"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/arbion/platform/services/api/internal/financial"
)

// oauthEscape implements RFC 5849/RFC 3986 encoding (spaces are %20, not +).
func oauthEscape(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func authorizationHeader(u *url.URL, key, consumerSecret, token, tokenSecret, nonce string, timestamp int64, extra url.Values) (string, error) {
	params, err := url.ParseQuery(u.RawQuery)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || timestamp <= 0 || nonce == "" {
		return "", failure(financial.InternalError)
	}
	for k := range params {
		if strings.HasPrefix(k, "oauth_") {
			return "", failure(financial.InternalError)
		}
	}
	oauth := url.Values{"oauth_consumer_key": {key}, "oauth_nonce": {nonce},
		"oauth_signature_method": {"HMAC-SHA1"}, "oauth_timestamp": {strconv.FormatInt(timestamp, 10)}}
	if token != "" {
		oauth.Set("oauth_token", token)
	}
	for k, v := range extra {
		if (k != "oauth_callback" && k != "oauth_verifier") || len(v) != 1 {
			return "", failure(financial.InternalError)
		}
		oauth[k] = v
	}
	for k, v := range oauth {
		params[k] = v
	}
	type pair struct{ key, value string }
	var pairs []pair
	for key, values := range params {
		for _, value := range values {
			pairs = append(pairs, pair{oauthEscape(key), oauthEscape(value)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key == pairs[j].key {
			return pairs[i].value < pairs[j].value
		}
		return pairs[i].key < pairs[j].key
	})
	var normalized []string
	for _, p := range pairs {
		normalized = append(normalized, p.key+"="+p.value)
	}
	host := strings.ToLower(u.Host)
	if strings.HasSuffix(host, ":443") {
		host = strings.TrimSuffix(host, ":443")
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	base := "GET&" + oauthEscape("https://"+host+path) + "&" + oauthEscape(strings.Join(normalized, "&"))
	mac := hmac.New(sha1.New, []byte(oauthEscape(consumerSecret)+"&"+oauthEscape(tokenSecret)))
	_, _ = mac.Write([]byte(base))
	oauth.Set("oauth_signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	var fields []string
	for key, values := range oauth {
		fields = append(fields, oauthEscape(key)+"=\""+oauthEscape(values[0])+"\"")
	}
	sort.Strings(fields)
	return "OAuth " + strings.Join(fields, ", "), nil
}
