CREATE TABLE follows (
  id uuid PRIMARY KEY,
  follower_id uuid NOT NULL,
  followee_id uuid NOT NULL,
  source text NOT NULL DEFAULT 'profile'
    CHECK (source IN ('profile', 'phone', 'x', 'cabal', 'feed', 'suggested', 'referral')),
  created_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,
  CHECK (follower_id <> followee_id)
);

CREATE UNIQUE INDEX follows_pair_live_idx ON follows (follower_id, followee_id) WHERE deleted_at IS NULL;

CREATE INDEX follows_followee_live_idx ON follows (followee_id, created_at DESC) WHERE deleted_at IS NULL;

CREATE INDEX follows_follower_live_idx ON follows (follower_id, created_at DESC) WHERE deleted_at IS NULL;
