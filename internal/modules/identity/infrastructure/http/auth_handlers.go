package http

import (
	"net/http"

	identitydomain "github.com/manuellysuzik/mesaposta-api/internal/modules/identity/domain"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/identity/application"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
)

// loginHandler returns a handler for POST /v1/auth/login. It authenticates
// the request's email/password via application.AuthenticateUser (which
// looks the user up directly via lookup, bypassing UnitOfWork/tenant --
// see AuthenticateUser's own doc comment for why a login attempt has no
// tenant established yet) and, on success, mints an access token via
// issuer.
//
// Converting the domain-level Role AuthenticateUser returns into a
// token.Role happens HERE, in infrastructure/http -- never in domain/ or
// application/, which must not depend on internal/platform/auth/token (see
// internal/modules/identity/domain.Role's own doc comment).
func loginHandler(lookup application.UserCredentialsLookup, issuer *token.Issuer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := decodeJSON[loginRequest](r)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		authed, err := application.AuthenticateUser(r.Context(), lookup, application.AuthenticateUserInput{
			Email:    req.Email,
			Password: req.Password,
		})
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		tokenRole := token.Role(string(authed.Role))
		if !tokenRole.Valid() {
			httpx.RespondError(w, r, errs.New(errs.Internal, "invalid_role", "authenticated user has an unknown role", nil))
			return
		}

		// No refresh-token flow yet (see application/refresh_session.go's
		// own doc comment on that gap): sessionID only needs to be a
		// syntactically valid, unique value to populate the "sid" claim --
		// it is not looked up anywhere yet.
		sessionID := identitydomain.NewID().String()

		accessToken, err := issuer.Issue(authed.UserID, authed.TenantID, tokenRole, sessionID)
		if err != nil {
			httpx.RespondError(w, r, err)
			return
		}

		httpx.RespondData(w, r, http.StatusOK, loginResponse{
			AccessToken: accessToken,
			ExpiresIn:   int64(token.AccessTokenTTL.Seconds()),
			Role:        string(authed.Role),
			UserID:      authed.UserID,
			TenantID:    authed.TenantID,
		}, nil)
	}
}
