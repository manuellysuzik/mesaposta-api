package application

import (
	"context"

	"github.com/manuellysuzik/mesaposta-api/internal/modules/ordering/domain"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/tenant"
)

// ResolveTableForMenuResult bundles everything the public
// GET .../cardapio/mesa/{code} handler needs to build its response: the
// resolved Table, the freshly minted TableSession, and the PLAINTEXT
// session token -- which, like every other opaque token in this codebase
// (see internal/platform/auth/refresh), exists ONLY in this return value
// and is never itself persisted; only its hash is, via
// domain.TableSession.TokenHash().
type ResolveTableForMenuResult struct {
	Table        domain.Table
	Session      domain.TableSession
	SessionToken string
}

// cardapioUnavailableErr is the SINGLE error every failure to resolve code
// into a usable table is reported as -- collapsing "no such table code"
// and "table code exists but the table is deactivated" into one
// indistinguishable response, mirroring
// internal/platform/httpx.NotFoundError's own documented "collapse
// distinct causes" pattern (see that function's doc comment) and
// internal/platform/auth/publictenant's identical treatment of an unknown
// vs. deactivated tenant slug. cause carries the real reason for
// server-side logs only -- it is never surfaced to the client (see
// internal/platform/httpx.ErrorBodyFor).
//
// This is THE security-critical behavior this module's own test suite
// (../infrastructure/httppublic) proves byte-for-byte: a request for a
// table code that does not exist and a request for a table code that
// exists but is inactive must produce IDENTICAL HTTP responses, so an
// unauthenticated caller can never use this endpoint to enumerate which
// table codes are real.
func cardapioUnavailableErr(cause error) error {
	return errs.New(errs.NotFound, "cardapio_indisponivel", "cardápio indisponível para esta mesa", cause)
}

// ResolveTableForMenu is the application-layer use case backing the public
// GET .../cardapio/mesa/{code} route: it looks up the Table identified by
// code within tenantID, refuses (via cardapioUnavailableErr) if it does
// not exist OR is not active, and -- only if the table is valid -- mints a
// brand-new TableSession bound to it, persisting the session and returning
// its plaintext token alongside the resolved Table.
func ResolveTableForMenu(ctx context.Context, uow domain.UnitOfWork, tenantID tenant.TenantID, code string) (ResolveTableForMenuResult, error) {
	var result ResolveTableForMenuResult

	err := uow.Do(ctx, func(ctx context.Context, r domain.Repositories) error {
		table, err := r.Tables().FindByCode(ctx, code)
		if err != nil {
			return cardapioUnavailableErr(err)
		}
		if !table.IsActive() {
			return cardapioUnavailableErr(nil)
		}

		session, plaintext, err := domain.NewTableSession(tenantID, table.ID())
		if err != nil {
			return err
		}
		if err := r.TableSessions().Insert(ctx, session); err != nil {
			return err
		}

		result = ResolveTableForMenuResult{Table: *table, Session: session, SessionToken: plaintext}
		return nil
	})
	if err != nil {
		return ResolveTableForMenuResult{}, err
	}
	return result, nil
}
