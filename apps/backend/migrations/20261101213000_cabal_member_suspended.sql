ALTER TABLE cabal_members ADD COLUMN suspended boolean NOT NULL DEFAULT false;

ALTER TABLE cabal_members ADD CONSTRAINT cabal_members_suspended_inert
  CHECK (NOT suspended OR (role = 'member' AND NOT can_vote));
