CREATE TABLE feed_objects (
  id uuid PRIMARY KEY,
  kind text NOT NULL CHECK (kind IN ('proposal', 'trade', 'price_move', 'cabal_created', 'member_joined')),
  ref_type text NOT NULL
    CHECK (ref_type IN ('proposals', 'swaps', 'asset_price_moves', 'cabals', 'cabal_members')),
  ref_id uuid NOT NULL,
  cabal_id uuid,
  cabal_name text,
  actor_id uuid,
  asset_id uuid,
  symbol text,
  title text NOT NULL,
  body text,
  payload jsonb NOT NULL,
  status text,
  comment_count int NOT NULL DEFAULT 0,
  search tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('simple', coalesce(symbol, '') || ' ' || coalesce(cabal_name, '')), 'A')
    || setweight(to_tsvector('english', title), 'B')
    || setweight(to_tsvector('english', coalesce(body, '')), 'C')
  ) STORED,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE (ref_type, ref_id, kind)
);

CREATE INDEX feed_objects_newest_idx ON feed_objects (created_at DESC, id DESC);

CREATE INDEX feed_objects_cabal_idx ON feed_objects (cabal_id, created_at DESC);

CREATE INDEX feed_objects_asset_idx ON feed_objects (asset_id, created_at DESC);

CREATE INDEX feed_objects_kind_idx ON feed_objects (kind, created_at DESC);

CREATE INDEX feed_objects_search_idx ON feed_objects USING gin (search);

CREATE TABLE feed_comments (
  id uuid PRIMARY KEY,
  feed_object_id uuid NOT NULL REFERENCES feed_objects (id),
  author_id uuid NOT NULL,
  parent_comment_id uuid,
  reply_to_user_id uuid,
  body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1000),
  created_at timestamptz NOT NULL,
  deleted_at timestamptz,
  deleted_by uuid,
  UNIQUE (feed_object_id, id),
  FOREIGN KEY (feed_object_id, parent_comment_id) REFERENCES feed_comments (feed_object_id, id)
);

CREATE INDEX feed_comments_item_idx ON feed_comments (feed_object_id, created_at);

CREATE TABLE feed_mutes (
  user_id uuid NOT NULL,
  target_type text NOT NULL CHECK (target_type IN ('kind', 'cabal', 'asset', 'user', 'item')),
  target_id text NOT NULL,
  label text,
  created_at timestamptz NOT NULL,
  PRIMARY KEY (user_id, target_type, target_id)
);
