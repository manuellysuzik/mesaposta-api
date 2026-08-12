-- 0007_community.sql
--
-- Backing tables for internal/modules/community's TWO SEPARATE aggregate
-- roots -- CommunityPost and Like -- plus the (append-only) Comment entity
-- that references a CommunityPost by id (see
-- internal/modules/community/domain/community_post.go and like.go's own
-- doc comments for why Like is deliberately a separate aggregate from
-- CommunityPost, not nested inside it), as persisted by
-- internal/modules/community/infrastructure/postgres. Follows the same
-- ownership/RLS convention as every migration since 0001_baseline.sql:
-- every table is owned by whichever administrative role runs migrations
-- (never mesaposta_app), RLS is enabled AND forced, and every policy is
-- named "tenant_isolation" with the exact same predicate used since
-- 0001_baseline.sql:
--
--     USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
--     WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid)
--
-- Cross-tenant foreign keys: community_posts carries a UNIQUE
-- (tenant_id, id) constraint, and both community_comments' and
-- community_likes' foreign keys back to it are composite --
-- FOREIGN KEY (tenant_id, post_id) REFERENCES community_posts (tenant_id, id)
-- -- instead of a bare FOREIGN KEY (post_id) REFERENCES community_posts (id).
-- A single-column FK would let a row reference a same-"id" row belonging to
-- a DIFFERENT tenant (ids are globally unique UUIDs, so nothing else would
-- stop that); the composite FK makes a cross-tenant reference physically
-- impossible to insert, not just something RLS happens to hide afterwards
-- -- see 0004_operations.sql's own comment for the identical rationale.
--
-- author_id (on community_posts and community_comments) and user_id (on
-- community_likes) are all bare uuid columns with NO foreign key, exactly
-- like 0004_operations.sql's own supplier_id: there is no Users/accounts
-- module or table in this phase for them to reference (see
-- internal/modules/community/infrastructure/http/currentuser.go's own doc
-- comment on the interim design decision this implies for where a user id
-- comes from). A future Users module can backfill the FK without a column
-- rename, the same deferred-FK pattern every other module in this
-- codebase already follows for a not-yet-existing referenced aggregate.
--
-- community_comments.text' CHECK (char_length(text) BETWEEN 1 AND 2000) is
-- the database-level mirror of
-- internal/modules/community/domain.CommentText's own 1-2000 rune bound --
-- a second, physical backstop behind that application-level validation,
-- matching every other enum/bound CHECK constraint in this codebase.

-- +goose Up

-- community_posts: a single post in a tenant's internal community feed
-- (see domain/community_post.go). Immutable once created -- no version
-- column, no UPDATE grant below -- mirroring 0004_operations.sql's
-- treatment of its own append-only tables (stock_movements, transactions).
CREATE TABLE community_posts (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    author_id uuid NOT NULL, -- no FK: no Users/accounts aggregate exists in this phase.
    content text NOT NULL CHECK (char_length(content) >= 1),
    created_at timestamptz NOT NULL,
    CONSTRAINT community_posts_tenant_id_id_key UNIQUE (tenant_id, id)
);

ALTER TABLE community_posts ENABLE ROW LEVEL SECURITY;
ALTER TABLE community_posts FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON community_posts
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT ON community_posts TO mesaposta_app;

-- community_comments: an append-only log of every comment made on a
-- CommunityPost (see domain/comment.go's own doc comment: Comment is not
-- an aggregate root of its own and is never updated or deleted). GRANT
-- SELECT, INSERT only -- deliberately omitting UPDATE and DELETE -- is
-- this table's own enforcement of that same immutability at the one layer
-- application code cannot talk its way around, mirroring
-- 0004_operations.sql's identical treatment of stock_movements and
-- 0006_crm.sql's identical treatment of loyalty_entries.
CREATE TABLE community_comments (
    id uuid PRIMARY KEY,
    tenant_id uuid NOT NULL,
    post_id uuid NOT NULL,
    author_id uuid NOT NULL, -- no FK: no Users/accounts aggregate exists in this phase.
    text text NOT NULL CHECK (char_length(text) BETWEEN 1 AND 2000),
    created_at timestamptz NOT NULL,
    CONSTRAINT community_comments_post_fk
        FOREIGN KEY (tenant_id, post_id) REFERENCES community_posts (tenant_id, id)
);

ALTER TABLE community_comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE community_comments FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON community_comments
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT ON community_comments TO mesaposta_app;

-- community_likes: the Like aggregate (see domain/like.go's own doc
-- comment on why it is a SEPARATE aggregate root from CommunityPost). Its
-- surrogate "id" primary key exists only so the row has one at all --
-- application code never reads it back, mirroring 0004_operations.sql's
-- purchase_order_items' own "value object, surrogate id" shape -- the
-- real, business-meaningful identity is the UNIQUE (tenant_id, post_id,
-- user_id) constraint below, which is what
-- internal/modules/community/infrastructure/postgres/like_repository.go's
-- "INSERT ... ON CONFLICT (tenant_id, post_id, user_id) DO NOTHING"
-- relies on to make PUT /v1/posts/{id}/curtidas/me idempotent. Unlike
-- community_posts/community_comments, this table DOES need a DELETE grant
-- (there is no UPDATE grant: a like is only ever inserted or deleted, never
-- modified in place) -- that is what makes
-- DELETE /v1/posts/{id}/curtidas/me possible at all.
CREATE TABLE community_likes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    post_id uuid NOT NULL,
    user_id uuid NOT NULL, -- no FK: no Users/accounts aggregate exists in this phase.
    created_at timestamptz NOT NULL,
    CONSTRAINT community_likes_post_fk
        FOREIGN KEY (tenant_id, post_id) REFERENCES community_posts (tenant_id, id),
    CONSTRAINT community_likes_tenant_post_user_key UNIQUE (tenant_id, post_id, user_id)
);

ALTER TABLE community_likes ENABLE ROW LEVEL SECURITY;
ALTER TABLE community_likes FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON community_likes
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

GRANT SELECT, INSERT, DELETE ON community_likes TO mesaposta_app;

-- +goose Down
DROP TABLE IF EXISTS community_likes;
DROP TABLE IF EXISTS community_comments;
DROP TABLE IF EXISTS community_posts;
