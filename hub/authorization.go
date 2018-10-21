package hub

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	jwt "github.com/dgrijalva/jwt-go"
)

// Claims contains Mercure's JWT claims
type claims struct {
	Mercure mercureClaim `json:"mercure"`
	jwt.StandardClaims
}

type mercureClaim struct {
	Publish   []string `json:"publish"`
	Subscribe []string `json:"subscribe"`
}

// Authorize validates the JWT that may be provided through an "Authorization" HTTP header or a "mercureAuthorization" cookie.
// It returns the claims contained in the token if it exists and is valid, nil if no token is provided (anonymous mode), and an error if the token is not valid.
func authorize(r *http.Request, jwtKey []byte, publishAllowedOrigins []string) (*claims, error) {
	authorizationHeaders, headerExists := r.Header["Authorization"]
	if headerExists {
		if len(authorizationHeaders) != 1 || len(authorizationHeaders[0]) < 48 || authorizationHeaders[0][:7] != "Bearer " {
			return nil, errors.New("Invalid \"Authorization\" HTTP header")
		}

		return validateJWT(authorizationHeaders[0][7:], jwtKey)
	}

	cookie, err := r.Cookie("mercureAuthorization")
	if err != nil {
		// Anonymous
		return nil, nil
	}

	// CSRF attacks cannot occurs when using safe methods
	if r.Method != "POST" {
		return validateJWT(cookie.Value, jwtKey)
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		// Try to extract the origin from the Referer, or return an error
		referer := r.Header.Get("Referer")
		if referer == "" {
			return nil, errors.New("An \"Origin\" or a \"Referer\" HTTP header must be present to use the cookie-based authorization mechanism")
		}

		u, err := url.Parse(referer)
		if err != nil {
			return nil, err
		}

		origin = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	}

	for _, allowedOrigin := range publishAllowedOrigins {
		if origin == allowedOrigin {
			return validateJWT(cookie.Value, jwtKey)
		}
	}

	return nil, fmt.Errorf("The origin \"%s\" is not allowed to post updates", origin)
}

// validateJWT validates that the provided JWT token is a valid Mercure token
func validateJWT(encodedToken string, key []byte) (*claims, error) {
	token, err := jwt.ParseWithClaims(encodedToken, &claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return key, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("Invalid JWT")
}

func authorizedTargets(claims *claims, publisher bool) (all bool, targets map[string]struct{}) {
	if claims == nil {
		return false, map[string]struct{}{}
	}

	var providedTargets []string
	if publisher {
		providedTargets = claims.Mercure.Publish
	} else {
		providedTargets = claims.Mercure.Subscribe
	}

	authorizedTargets := make(map[string]struct{}, len(providedTargets))
	for _, target := range providedTargets {
		if target == "*" {
			return true, nil
		}

		authorizedTargets[target] = struct{}{}
	}

	return false, authorizedTargets
}
