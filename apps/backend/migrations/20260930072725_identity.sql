CREATE TABLE users (
  id uuid PRIMARY KEY,
  privy_user_id text NOT NULL UNIQUE,
  handle text CHECK (handle ~ '^[a-z0-9_]{3,20}$'),
  handle_changed_at timestamptz,
  display_name text NOT NULL DEFAULT '',
  photo_url text,
  email text,
  login_provider text NOT NULL CHECK (login_provider IN ('sms', 'email', 'apple', 'google')),
  phone_e164 text,
  phone_hash bytea UNIQUE CHECK (octet_length(phone_hash) = 32),
  phone_verified_at timestamptz,
  x_user_id text UNIQUE,
  x_username text,
  x_linked_at timestamptz,
  auth_state text NOT NULL DEFAULT 'CREATED'
    CHECK (auth_state IN ('CREATED', 'AWAITING_PHONE', 'AWAITING_SOCIALS', 'ONBOARDING_COMPLETED')),
  auth_state_changed_at timestamptz NOT NULL,
  account_status text NOT NULL DEFAULT 'active'
    CHECK (account_status IN ('active', 'suspended', 'banned', 'deleted')),
  first_deposit_at timestamptz,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  deleted_at timestamptz
);

CREATE UNIQUE INDEX users_handle_key ON users (handle);

CREATE INDEX users_auth_state_idx ON users (auth_state);

CREATE INDEX users_account_status_idx ON users (account_status);

CREATE INDEX users_x_username_lower_idx ON users (lower(x_username));

CREATE TABLE user_wallets (
  user_id uuid PRIMARY KEY REFERENCES users (id),
  privy_wallet_id text NOT NULL UNIQUE,
  address text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL
);
