CREATE TABLE referral_codes (
  code text PRIMARY KEY CHECK (code ~ '^[23456789abcdefghjkmnpqrstuvwxyz]{8}$'),
  user_id uuid NOT NULL UNIQUE,
  created_at timestamptz NOT NULL
);

CREATE TABLE referrals (
  id uuid PRIMARY KEY,
  referrer_id uuid NOT NULL,
  referee_id uuid NOT NULL UNIQUE,
  code text NOT NULL,
  code_kind text NOT NULL CHECK (code_kind IN ('random', 'handle')),
  source text NOT NULL CHECK (source IN ('universal_link', 'clipboard', 'manual')),
  status text NOT NULL CHECK (status IN ('attributed', 'qualified', 'rejected')),
  reject_reason text,
  created_at timestamptz NOT NULL,
  qualified_at timestamptz,
  CHECK (referrer_id <> referee_id)
);

CREATE INDEX referrals_referrer_created_idx ON referrals (referrer_id, created_at);

CREATE TABLE referral_clicks (
  code text NOT NULL,
  day date NOT NULL,
  clicks int NOT NULL DEFAULT 0,
  PRIMARY KEY (code, day)
);
