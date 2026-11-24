-- EXAMPLE RESOURCE. "items" demonstrates CRUD + search + filters + pagination +
-- media + a public SEO page. Copy this pattern for your own resources
-- (see .claude/skills/add-resource), or delete it with its package internal/items.

CREATE TABLE items (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    owner_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title      text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    body       text NOT NULL DEFAULT '' CHECK (char_length(body) <= 20000),
    status     text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    cover_id   uuid REFERENCES media (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- full-text search vector maintained by Postgres (no search server needed)
    search     tsvector GENERATED ALWAYS AS (
                   setweight(to_tsvector('simple', title), 'A') ||
                   setweight(to_tsvector('simple', body), 'B')) STORED
);
CREATE INDEX items_owner_created ON items (owner_id, created_at DESC, id DESC);
CREATE INDEX items_published ON items (created_at DESC, id DESC) WHERE status = 'published';
CREATE INDEX items_search ON items USING gin (search);
CREATE INDEX items_title_trgm ON items USING gin (title gin_trgm_ops); -- fast ILIKE '%term%'
