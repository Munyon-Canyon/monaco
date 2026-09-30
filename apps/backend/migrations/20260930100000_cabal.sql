CREATE TABLE cabals (
  id uuid PRIMARY KEY,
  name text NOT NULL CHECK (char_length(name) BETWEEN 3 AND 40),
  picture_url text,
  creator_id uuid NOT NULL REFERENCES users (id),
  join_mode text NOT NULL CHECK (join_mode IN ('open', 'request')),
  voter_mode text NOT NULL CHECK (voter_mode IN ('all', 'list')),
  threshold text NOT NULL CHECK (threshold IN ('majority', 'unanimous')),
  proposal_expiry_seconds int NOT NULL CHECK (proposal_expiry_seconds IN (3600, 86400, 604800)),
  slippage_bps int NOT NULL DEFAULT 100 CHECK (slippage_bps BETWEEN 1 AND 300),
  invite_code text NOT NULL UNIQUE CHECK (invite_code ~ '^[0-9A-HJKMNP-TV-Z]{10}$'),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'banned')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);

CREATE INDEX cabals_name_lower_idx ON cabals (lower(name) text_pattern_ops);

CREATE TABLE cabal_members (
  cabal_id uuid NOT NULL REFERENCES cabals (id),
  user_id uuid NOT NULL REFERENCES users (id),
  role text NOT NULL CHECK (role IN ('creator', 'member')),
  can_vote boolean NOT NULL,
  joined_at timestamptz NOT NULL,
  PRIMARY KEY (cabal_id, user_id),
  CONSTRAINT cabal_members_creator_votes CHECK (role <> 'creator' OR can_vote)
);

CREATE INDEX cabal_members_user_id_idx ON cabal_members (user_id);

CREATE UNIQUE INDEX cabal_members_one_creator_key ON cabal_members (cabal_id) WHERE role = 'creator';

CREATE TABLE cabal_access_requests (
  id uuid PRIMARY KEY,
  cabal_id uuid NOT NULL REFERENCES cabals (id),
  user_id uuid NOT NULL REFERENCES users (id),
  direction text NOT NULL CHECK (direction IN ('request', 'invite')),
  invited_by uuid REFERENCES users (id),
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'denied', 'revoked', 'expired')),
  expires_at timestamptz,
  decided_by uuid REFERENCES users (id),
  created_at timestamptz NOT NULL,
  decided_at timestamptz,
  CONSTRAINT cabal_access_requests_direction_shape CHECK (
    (direction = 'request' AND invited_by IS NULL AND expires_at IS NULL)
    OR (direction = 'invite' AND invited_by IS NOT NULL AND expires_at IS NOT NULL)
  ),
  CONSTRAINT cabal_access_requests_requests_do_not_expire CHECK (status <> 'expired' OR direction = 'invite')
);

CREATE UNIQUE INDEX cabal_access_requests_pending_key ON cabal_access_requests (cabal_id, user_id)
  WHERE status = 'pending';

CREATE INDEX cabal_access_requests_invite_expiry_idx ON cabal_access_requests (expires_at)
  WHERE status = 'pending' AND direction = 'invite';

CREATE TABLE treasury_wallets (
  cabal_id uuid PRIMARY KEY REFERENCES cabals (id),
  privy_wallet_id text NOT NULL UNIQUE,
  address text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL
);
