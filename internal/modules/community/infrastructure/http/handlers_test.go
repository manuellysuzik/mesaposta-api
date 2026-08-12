package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	communityhttp "github.com/manuellysuzik/mesaposta-api/internal/modules/community/infrastructure/http"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/community/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/auth/token"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// This suite exercises the community module's HTTP handlers end-to-end
// through the REAL auth.Authenticate middleware (rather than manually
// injecting a tenant.TenantID into the request context, as
// internal/modules/crm's own http tests do): unlike crm, this module's
// like/unlike/comment handlers need the authenticated caller's own
// identity (see currentuser.go), which only flows through context via
// auth.Authenticate -- there is no exported way to inject
// token.Claims into a context from outside the auth package (R6-style
// encapsulation: auth.withClaims is unexported), so a real, signed JWT is
// the only way to exercise that path from this package.

const testJWTSecret = "test-secret-at-least-32-bytes-long-for-hs256"
const testIssuer = "mesaposta-api-test"
const testAudience = "mesaposta-api-test"
const testKID = "test-kid"

var testTenantIDStr = "cccccccc-dddd-eeee-ffff-000000000000"

func mustHTTPTenantID(t *testing.T) tenant.TenantID {
	t.Helper()
	id, err := tenant.Parse(testTenantIDStr)
	if err != nil {
		t.Fatalf("tenant.Parse(%q): %v", testTenantIDStr, err)
	}
	return id
}

func mustTestIssuer(t *testing.T) *token.Issuer {
	t.Helper()
	i, err := token.NewIssuer(testKID, []byte(testJWTSecret), testIssuer, testAudience)
	if err != nil {
		t.Fatalf("token.NewIssuer: %v", err)
	}
	return i
}

// mustAccessToken mints a signed access token whose "sub" claim is subject
// (typically a UUID string this module's currentUserID can parse) and
// whose tenant_id claim is tenantIDStr.
func mustAccessToken(t *testing.T, subject, tenantIDStr string) string {
	t.Helper()
	tok, err := mustTestIssuer(t).Issue(subject, tenantIDStr, token.RoleGestor, "session-"+subject)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return tok
}

// newTestRouter builds a chi.Router with auth.Authenticate wired in front
// of the community module (mirroring how internal/platform/app.NewApp
// mounts every module in production) and the module mounted against uow,
// ready for httptest-based handler tests.
func newTestRouter(uow *fakeUnitOfWork) chi.Router {
	r := chi.NewRouter()
	r.Use(auth.Authenticate(mustTestVerifier()))
	communityhttp.Mount(r, uow)
	return r
}

// mustTestVerifier builds the token.Verifier every test router in this
// package authenticates against. It panics (rather than taking a
// *testing.T) so it can be called from newTestRouter, which itself is
// invoked before some call sites have a convenient *testing.T on hand;
// this mirrors how this codebase's own domain packages panic in their
// "must never fail in practice" constructors (e.g. domain.NewID).
func mustTestVerifier() *token.Verifier {
	v, err := token.NewVerifier(map[string][]byte{testKID: []byte(testJWTSecret)}, testIssuer, testAudience)
	if err != nil {
		panic(err)
	}
	return v
}

func decodeEnvelope(t *testing.T, body *bytes.Buffer) map[string]json.RawMessage {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding response envelope: %v (body=%s)", err, body.String())
	}
	return envelope
}

func mustHTTPPost(t *testing.T, tenantID tenant.TenantID) *domain.CommunityPost {
	t.Helper()
	post, err := domain.NewCommunityPost(domain.NewID(), tenantID, domain.NewID(), "Post HTTP de teste")
	if err != nil {
		t.Fatalf("NewCommunityPost: %v", err)
	}
	return &post
}

// authedRequest builds req carrying a valid "Authorization: Bearer ..."
// header for subject/tenantID.
func authedRequest(req *http.Request, subject, tenantIDStr string, t *testing.T) *http.Request {
	req.Header.Set("Authorization", "Bearer "+mustAccessToken(t, subject, tenantIDStr))
	return req
}

// --- GET /posts/{id} ---------------------------------------------------------

func TestGetPostHandler_Found_Returns200(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	post := mustHTTPPost(t, tenantID)
	state := newFakeState()
	state.posts[post.ID()] = post
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/posts/"+post.ID().String(), nil)
	req = authedRequest(req, domain.NewID().String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	envelope := decodeEnvelope(t, w.Body)
	if envelope["error"] != nil && string(envelope["error"]) != "null" {
		t.Errorf("envelope.error = %s, want null", envelope["error"])
	}
}

func TestGetPostHandler_NotFound_Returns404(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/posts/"+domain.NewID().String(), nil)
	req = authedRequest(req, domain.NewID().String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestGetPostHandler_MalformedID_Returns400(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/posts/not-a-uuid", nil)
	req = authedRequest(req, domain.NewID().String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestGetPostHandler_NoAuthorizationHeader_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodGet, "/posts/"+domain.NewID().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// --- POST /posts --------------------------------------------------------------

func TestCreatePostHandler_Valid_Returns201(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"content": "Novo post da equipe"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/posts/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authedRequest(req, domain.NewID().String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	if snapshot := uow.snapshot(); len(snapshot.posts) != 1 {
		t.Errorf("posts persisted = %d, want 1", len(snapshot.posts))
	}
}

func TestCreatePostHandler_EmptyContent_Returns400(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"content": "   "})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/posts/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authedRequest(req, domain.NewID().String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestCreatePostHandler_NonUUIDSubject_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"content": "conteudo valido"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/posts/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authedRequest(req, "not-a-uuid-subject", testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

// --- POST /posts/{id}/comments -----------------------------------------------

func TestCommentOnPostHandler_Valid_Returns201(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	post := mustHTTPPost(t, tenantID)
	state := newFakeState()
	state.posts[post.ID()] = post
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"text": "excelente!"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/posts/"+post.ID().String()+"/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authedRequest(req, domain.NewID().String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusCreated, w.Body.String())
	}
	if snapshot := uow.snapshot(); len(snapshot.comments) != 1 {
		t.Errorf("comments persisted = %d, want 1", len(snapshot.comments))
	}
}

func TestCommentOnPostHandler_PostNotFound_Returns404(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	body, err := json.Marshal(map[string]any{"text": "comentario"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/posts/"+domain.NewID().String()+"/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authedRequest(req, domain.NewID().String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestCommentOnPostHandler_TooLongText_Returns400(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	post := mustHTTPPost(t, tenantID)
	state := newFakeState()
	state.posts[post.ID()] = post
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	longText := make([]byte, 2001)
	for i := range longText {
		longText[i] = 'a'
	}
	body, err := json.Marshal(map[string]any{"text": string(longText)})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/posts/"+post.ID().String()+"/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = authedRequest(req, domain.NewID().String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// --- PUT /posts/{id}/curtidas/me ---------------------------------------------

func TestLikePostHandler_Valid_Returns200(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	post := mustHTTPPost(t, tenantID)
	state := newFakeState()
	state.posts[post.ID()] = post
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	userID := domain.NewID()
	req := httptest.NewRequest(http.MethodPut, "/posts/"+post.ID().String()+"/curtidas/me", nil)
	req = authedRequest(req, userID.String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if snapshot := uow.snapshot(); len(snapshot.likes) != 1 {
		t.Errorf("likes persisted = %d, want 1", len(snapshot.likes))
	}
}

// TestLikePostHandler_CalledTwice_IsIdempotent_BothReturn200 is this
// module's headline HTTP-level acceptance criterion for liking: a repeated
// PUT from the same user on the same post must succeed both times (never a
// 409), and must persist exactly one like.
func TestLikePostHandler_CalledTwice_IsIdempotent_BothReturn200(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	post := mustHTTPPost(t, tenantID)
	state := newFakeState()
	state.posts[post.ID()] = post
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	userID := domain.NewID()

	req1 := httptest.NewRequest(http.MethodPut, "/posts/"+post.ID().String()+"/curtidas/me", nil)
	req1 = authedRequest(req1, userID.String(), testTenantIDStr, t)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest(http.MethodPut, "/posts/"+post.ID().String()+"/curtidas/me", nil)
	req2 = authedRequest(req2, userID.String(), testTenantIDStr, t)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w1.Code != http.StatusOK {
		t.Fatalf("1st PUT: status = %d, want %d (body=%s)", w1.Code, http.StatusOK, w1.Body.String())
	}
	if w2.Code != http.StatusOK {
		t.Fatalf("2nd PUT: status = %d, want %d (body=%s)", w2.Code, http.StatusOK, w2.Body.String())
	}
	if snapshot := uow.snapshot(); len(snapshot.likes) != 1 {
		t.Errorf("likes persisted = %d, want exactly 1 (no duplicate)", len(snapshot.likes))
	}
}

func TestLikePostHandler_PostNotFound_Returns404(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodPut, "/posts/"+domain.NewID().String()+"/curtidas/me", nil)
	req = authedRequest(req, domain.NewID().String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// --- DELETE /posts/{id}/curtidas/me -------------------------------------------

func TestUnlikePostHandler_ExistingLike_Returns200(t *testing.T) {
	tenantID := mustHTTPTenantID(t)
	post := mustHTTPPost(t, tenantID)
	userID := domain.NewID()
	like, err := domain.NewLike(tenantID, post.ID(), userID)
	if err != nil {
		t.Fatalf("NewLike: %v", err)
	}
	state := newFakeState()
	state.posts[post.ID()] = post
	state.likes[likeKey{postID: post.ID(), userID: userID}] = like
	uow := newFakeUnitOfWork(state)
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodDelete, "/posts/"+post.ID().String()+"/curtidas/me", nil)
	req = authedRequest(req, userID.String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
	if snapshot := uow.snapshot(); len(snapshot.likes) != 0 {
		t.Errorf("likes persisted = %d, want 0 after unlike", len(snapshot.likes))
	}
}

// TestUnlikePostHandler_NeverLiked_IsIdempotent_Returns200 is this module's
// headline HTTP-level acceptance criterion for unliking: DELETE on a like
// that never existed must still succeed (never a 404/409).
func TestUnlikePostHandler_NeverLiked_IsIdempotent_Returns200(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodDelete, "/posts/"+domain.NewID().String()+"/curtidas/me", nil)
	req = authedRequest(req, domain.NewID().String(), testTenantIDStr, t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestUnlikePostHandler_NoAuthorizationHeader_Returns401(t *testing.T) {
	uow := newFakeUnitOfWork(newFakeState())
	r := newTestRouter(uow)

	req := httptest.NewRequest(http.MethodDelete, "/posts/"+domain.NewID().String()+"/curtidas/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}
