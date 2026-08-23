package http

// loginRequest is the request payload for POST /v1/auth/login.
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// loginResponse is the response payload for a successful POST
// /v1/auth/login: just enough for the frontend to attach the access token
// to subsequent requests and know when it expires.
type loginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	Role        string `json:"role"`
	UserID      string `json:"user_id"`
	TenantID    string `json:"tenant_id"`
}
